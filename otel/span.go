package otel

import (
	"context"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type spanImpl struct {
	ctx            context.Context
	span           trace.Span
	metrics        *metricRecorder
	name           string
	started        time.Time
	attrs          []glk.Attribute
	status         glk.SpanStatus
	serviceMetrics bool
}

func (s *spanImpl) End() {
	if s.serviceMetrics {
		s.metrics.recordServiceSpan(s.ctx, s.name, s.status, time.Since(s.started), s.attrs)
	}
	s.span.End()
}

func (s *spanImpl) SetError(err error) {
	if err == nil {
		return
	}
	s.status = glk.SpanStatusError
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, err.Error())
}

func (s *spanImpl) SetStatus(code glk.SpanStatus, message string) {
	s.status = code
	s.span.SetStatus(mapStatus(code), message)
}

func (s *spanImpl) SetAttributes(attrs ...glk.Attribute) {
	s.attrs = append(s.attrs, attrs...)
	s.span.SetAttributes(mapAttributes(attrs)...)
}

func (s *spanImpl) AddEvent(name string, attrs ...glk.Attribute) {
	s.span.AddEvent(name, trace.WithAttributes(mapAttributes(attrs)...))
}
