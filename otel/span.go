package otel

import (
	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type spanImpl struct {
	span trace.Span
}

func (s spanImpl) End() {
	s.span.End()
}

func (s spanImpl) SetError(err error) {
	if err == nil {
		return
	}
	s.span.RecordError(err)
	s.span.SetStatus(codes.Error, err.Error())
}

func (s spanImpl) SetStatus(code glk.SpanStatus, message string) {
	s.span.SetStatus(mapStatus(code), message)
}

func (s spanImpl) SetAttributes(attrs ...glk.Attribute) {
	s.span.SetAttributes(mapAttributes(attrs)...)
}

func (s spanImpl) AddEvent(name string, attrs ...glk.Attribute) {
	s.span.AddEvent(name, trace.WithAttributes(mapAttributes(attrs)...))
}
