package golitekit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

type trackerKeyType int

const (
	trackerKey trackerKeyType = iota
)

type serviceTracker struct {
	name      string
	started   bool
	startTime time.Time
	cost      time.Duration
}

type Tracker struct {
	name      string
	started   bool
	startTime time.Time
	totalCost time.Duration

	stack    []*serviceTracker
	services map[string]*serviceTracker

	logID string

	mu sync.Mutex
}

func GetTracker(ctx context.Context) *Tracker {
	tracker := ctx.Value(trackerKey)
	if tr, ok := tracker.(*Tracker); ok {
		return tr
	}
	return nil
}

func WithTracker(ctx context.Context) context.Context {
	// Fast path: ctx is a pooled *glkContext — initialize the embedded Tracker
	// directly without any context.WithValue allocation.
	if gctx, ok := ctx.(*glkContext); ok {
		tr := &gctx.tracker
		if !tr.started {
			tr.name = "self"
			tr.started = true
			tr.startTime = time.Now()
			tr.logID = generateLogID()
			if tr.services == nil {
				tr.services = make(map[string]*serviceTracker)
			}
		}
		return ctx
	}

	// Slow path: fallback for non-pooled contexts (e.g., tests without the router).
	tracker := GetTracker(ctx)
	if tracker == nil {
		tracker = &Tracker{
			name:      "self",
			started:   true,
			startTime: time.Now(),
			logID:     generateLogID(),
			services:  make(map[string]*serviceTracker),
		}
		return context.WithValue(ctx, trackerKey, tracker)
	}

	return ctx
}

func generateLogID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000")))[:16]
	}
	return hex.EncodeToString(b)
}

func (t *Tracker) LogID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.logID
}

func (t *Tracker) SetLogID(logID string) {
	if logID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logID = logID
}

func (s *serviceTracker) start() {
	if !s.started {
		s.started = true
		s.startTime = time.Now()
	}
}

func (s *serviceTracker) end() {
	if s.started {
		s.cost = time.Since(s.startTime)
		s.started = false
	}
}

func (t *Tracker) Start(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	st := &serviceTracker{
		name: name,
	}
	st.start()

	if len(t.stack) > 0 {
		t.stack[len(t.stack)-1].end()
	}

	// If this name is already tracked, generate a unique key so that both
	// entries are preserved in t.services and neither timing is lost.
	uniqueName := name
	if _, exists := t.services[uniqueName]; exists {
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s_%d", name, i)
			if _, exists := t.services[candidate]; !exists {
				uniqueName = candidate
				break
			}
		}
		st.name = uniqueName
	}

	t.stack = append(t.stack, st)
	t.services[uniqueName] = st
}

func (t *Tracker) End() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.stack) == 0 {
		return
	}

	t.stack[len(t.stack)-1].end()
	t.stack = t.stack[:len(t.stack)-1]
	if len(t.stack) > 0 {
		t.stack[len(t.stack)-1].start()
	}
}

func (t *Tracker) LogTracker(ctx context.Context) {
	t.mu.Lock()
	totalCost := time.Since(t.startTime)
	t.totalCost = totalCost
	logID := t.logID
	// Copy service entries under the lock to avoid racing with Start().
	services := make([]*serviceTracker, 0, len(t.services))
	for _, s := range t.services {
		services = append(services, s)
	}
	t.mu.Unlock()

	selfCost := totalCost
	logger.AddInfo(ctx, "logid", logID)

	for _, s := range services {
		selfCost -= s.cost
		logger.AddInfo(ctx, s.name+"_t", s.cost.Milliseconds())
	}

	// Clamp to zero: overlapping timers or clock precision can produce a small
	// negative value, which would be misleading in the log.
	if selfCost < 0 {
		selfCost = 0
	}

	logger.AddInfo(ctx, "all_t", totalCost.Milliseconds())
	logger.AddInfo(ctx, "self_t", selfCost.Milliseconds())
}

// resetForPool clears all request-scoped state so the embedded Tracker inside
// a pooled glkContext can be reused for the next request.
// Must only be called from glkContext.release() when no goroutine holds t.mu.
func (t *Tracker) resetForPool() {
	t.name = ""
	t.started = false
	t.startTime = time.Time{}
	t.totalCost = 0
	t.stack = t.stack[:0]
	for k := range t.services {
		delete(t.services, k)
	}
	t.logID = ""
}
