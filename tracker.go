package golite

import (
	"context"
	"github/hsj/golite/logger"
	"time"
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
			services:  make(map[string]*serviceTracker),
		}
	}

	return context.WithValue(ctx, trackerKey, tracker)
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
	if len(t.stack) > 0 {
		t.stack[len(t.stack)-1].end()
	}
	t.stack = t.stack[:len(t.stack)-1]
	if len(t.stack) > 0 {
		t.stack[len(t.stack)-1].start()
	}
}

func (t *Tracker) LogTracker(ctx context.Context) {
	t.totalCost = time.Since(t.startTime)
	selfCost := t.totalCost
	for _, s := range t.services {
		selfCost -= s.cost
		logger.AddInfo(ctx, s.name+"_t", s.cost.Milliseconds())
	}
	logger.AddInfo(ctx, "all_t", t.totalCost.Milliseconds())
	logger.AddInfo(ctx, "self_t", selfCost.Milliseconds())
}
