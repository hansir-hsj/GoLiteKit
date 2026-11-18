package golitekit

import "context"

type SpanStatus int

const (
	SpanStatusUnset SpanStatus = iota
	SpanStatusOK
	SpanStatusError
)

type AttributeValue struct {
	value any
}

func (v AttributeValue) AsInterface() any {
	return v.value
}

type Attribute struct {
	Key   string
	Value AttributeValue
}

func StringAttr(key, value string) Attribute {
	return Attribute{Key: key, Value: AttributeValue{value: value}}
}

func IntAttr(key string, value int) Attribute {
	return Attribute{Key: key, Value: AttributeValue{value: value}}
}

func BoolAttr(key string, value bool) Attribute {
	return Attribute{Key: key, Value: AttributeValue{value: value}}
}

func FloatAttr(key string, value float64) Attribute {
	return Attribute{Key: key, Value: AttributeValue{value: value}}
}

type Span interface {
	End()
	SetError(error)
	SetStatus(code SpanStatus, message string)
	SetAttributes(...Attribute)
	AddEvent(name string, attrs ...Attribute)
}

type Observer interface {
	StartSpan(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span)
}

type observerContextKey struct{}

type noopSpan struct{}

func (noopSpan) End() {}

func (noopSpan) SetError(error) {}

func (noopSpan) SetStatus(code SpanStatus, message string) {}

func (noopSpan) SetAttributes(...Attribute) {}

func (noopSpan) AddEvent(name string, attrs ...Attribute) {}

func WithObserverContext(ctx context.Context, observer Observer) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, observerContextKey{}, observer)
}

func ObserverFromContext(ctx context.Context) Observer {
	observer, _ := ctx.Value(observerContextKey{}).(Observer)
	return observer
}

// StartSpan starts a span with the request's observer, or returns a no-op span.
// name must be a low-cardinality constant (e.g. "load user", not
// "load user 42"): observers may use it as a metric label.
func StartSpan(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span) {
	observer := ObserverFromContext(ctx)
	if observer == nil {
		return ctx, noopSpan{}
	}
	return observer.StartSpan(ctx, name, attrs...)
}
