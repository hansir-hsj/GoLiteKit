package golitekit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	// tracking requests, each request is unique
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

	t.stack = append(t.stack, st)
	t.services[name] = st
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
	t.totalCost = time.Since(t.startTime)
	selfCost := t.totalCost

	// add logid
	logger.AddInfo(ctx, "logid", t.LogID())

	for _, s := range t.services {
		selfCost -= s.cost
		logger.AddInfo(ctx, s.name+"_t", s.cost.Milliseconds())
	}

	logger.AddInfo(ctx, "all_t", t.totalCost.Milliseconds())
	logger.AddInfo(ctx, "self_t", selfCost.Milliseconds())
}
