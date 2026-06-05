package golitekit

import (
	"context"
	"errors"
	"testing"
)

type fakeObserverKey struct{}

type fakeObserver struct {
	spans []*fakeSpan
}

type fakeSpan struct {
	name       string
	parentName string
	ended      bool
	err        error
	status     SpanStatus
	statusMsg  string
	attrs      []Attribute
	events     []string
}

func (o *fakeObserver) StartSpan(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span) {
	span := &fakeSpan{name: name, attrs: append([]Attribute(nil), attrs...)}
	if parent, ok := ctx.Value(fakeObserverKey{}).(*fakeSpan); ok {
		span.parentName = parent.name
	}
	o.spans = append(o.spans, span)
	return context.WithValue(ctx, fakeObserverKey{}, span), span
}

func (s *fakeSpan) End() {
	s.ended = true
}

func (s *fakeSpan) SetError(err error) {
	s.err = err
	s.status = SpanStatusError
	if err != nil {
		s.statusMsg = err.Error()
	}
}

func (s *fakeSpan) SetStatus(code SpanStatus, message string) {
	s.status = code
	s.statusMsg = message
}

func (s *fakeSpan) SetAttributes(attrs ...Attribute) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *fakeSpan) AddEvent(name string, attrs ...Attribute) {
	s.events = append(s.events, name)
}

func TestStartSpanWithoutObserverReturnsOriginalContextAndNoopSpan(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := StartSpan(ctx, "operation", StringAttr("component", "test"))

	if spanCtx != ctx {
		t.Fatal("expected original context without observer")
	}
	if span == nil {
		t.Fatal("expected no-op span")
	}

	span.End()
	span.SetError(errors.New("boom"))
	span.SetStatus(SpanStatusOK, "ok")
	span.SetAttributes(IntAttr("count", 1))
	span.AddEvent("event", BoolAttr("flag", true))
}

func TestStartSpanWithContextObserverCreatesRootAndChildSpans(t *testing.T) {
	observer := &fakeObserver{}
	ctx := WithObserverContext(context.Background(), observer)
	if ObserverFromContext(ctx) != observer {
		t.Fatal("expected observer from context")
	}

	rootCtx, root := StartSpan(ctx, "root", StringAttr("route", "/test"))
	childCtx, child := StartSpan(rootCtx, "child")

	rootSpan := root.(*fakeSpan)
	childSpan := child.(*fakeSpan)

	if childCtx == rootCtx {
		t.Fatal("expected observer to propagate child span context")
	}
	if len(observer.spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(observer.spans))
	}
	if rootSpan.name != "root" || rootSpan.parentName != "" {
		t.Fatalf("expected root span without parent, got name=%q parent=%q", rootSpan.name, rootSpan.parentName)
	}
	if childSpan.name != "child" || childSpan.parentName != "root" {
		t.Fatalf("expected child span with root parent, got name=%q parent=%q", childSpan.name, childSpan.parentName)
	}
}

func TestSpanSetErrorMarksErrorStatus(t *testing.T) {
	span := &fakeSpan{}
	err := errors.New("failed")

	span.SetError(err)

	if span.err != err {
		t.Fatal("expected error to be preserved")
	}
	if span.status != SpanStatusError {
		t.Fatalf("expected error status, got %v", span.status)
	}
	if span.statusMsg != "failed" {
		t.Fatalf("expected error message, got %q", span.statusMsg)
	}
}

func TestAttributeConstructorsPreserveValues(t *testing.T) {
	tests := []struct {
		name string
		attr Attribute
		want any
	}{
		{name: "string", attr: StringAttr("name", "alice"), want: "alice"},
		{name: "int", attr: IntAttr("count", 3), want: 3},
		{name: "bool", attr: BoolAttr("enabled", true), want: true},
		{name: "float", attr: FloatAttr("ratio", 1.5), want: 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.attr.Value.AsInterface() != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, tt.attr.Value.AsInterface())
			}
		})
	}
}

func TestWithObserverContextNilReturnsOriginalContext(t *testing.T) {
	ctx := context.Background()

	got := WithObserverContext(ctx, nil)

	if got != ctx {
		t.Fatal("expected original context when observer is nil")
	}
}
