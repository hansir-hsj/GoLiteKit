package otel

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// spanImpl is safe for concurrent use. End is idempotent: only the first call
// ends the span and records service metrics.
type spanImpl struct {
	ctx            context.Context
	span           trace.Span
	metrics        *metricRecorder
	name           string
	started        time.Time
	serviceMetrics bool
	ended          atomic.Bool

	mu     sync.Mutex
	attrs  []glk.Attribute
	status glk.SpanStatus
}

func (s *spanImpl) End() {
	if !s.ended.CompareAndSwap(false, true) {
		return
	}
	if s.serviceMetrics {
		s.mu.Lock()
		status := s.status
		attrs := append([]glk.Attribute(nil), s.attrs...)
		s.mu.Unlock()
		s.metrics.recordServiceSpan(s.ctx, s.name, status, time.Since(s.started), attrs)
	}
	s.span.End()
}

func (s *spanImpl) SetError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.status = glk.SpanStatusError
	s.mu.Unlock()
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, err.Error())
}

func (s *spanImpl) SetStatus(code glk.SpanStatus, message string) {
	s.mu.Lock()
	s.status = code
	s.mu.Unlock()
	s.span.SetStatus(mapStatus(code), message)
}

func (s *spanImpl) SetAttributes(attrs ...glk.Attribute) {
	s.mu.Lock()
	s.attrs = append(s.attrs, attrs...)
	s.mu.Unlock()
	s.span.SetAttributes(mapAttributes(attrs)...)
}

func (s *spanImpl) AddEvent(name string, attrs ...glk.Attribute) {
	s.span.AddEvent(name, trace.WithAttributes(mapAttributes(attrs)...))
}
