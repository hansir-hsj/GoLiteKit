package otel

import (
	"context"
	"errors"
	"testing"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestObserverCreatesRootAndChildSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider), WithServiceName("test-service"))

	ctx, root := observer.StartSpan(context.Background(), "root", glk.StringAttr("root.attr", "value"))
	_, child := observer.StartSpan(ctx, "child", glk.IntAttr("child.attr", 42))
	child.End()
	root.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported %d spans, want 2", len(spans))
	}

	childSpan := spans[0]
	rootSpan := spans[1]
	if rootSpan.Name != "root" {
		t.Fatalf("root span name = %q, want root", rootSpan.Name)
	}
	if childSpan.Name != "child" {
		t.Fatalf("child span name = %q, want child", childSpan.Name)
	}
	if childSpan.Parent.SpanID() != rootSpan.SpanContext.SpanID() {
		t.Fatalf("child parent span ID = %s, want %s", childSpan.Parent.SpanID(), rootSpan.SpanContext.SpanID())
	}
	if got := attributeValue(rootSpan, "root.attr"); got != "value" {
		t.Fatalf("root.attr = %v, want value", got)
	}
	if got := attributeValue(childSpan, "child.attr"); got != int64(42) {
		t.Fatalf("child.attr = %v, want 42", got)
	}
}

func TestSpanSetErrorRecordsError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))

	_, span := observer.StartSpan(context.Background(), "operation")
	span.SetError(errors.New("boom"))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	got := spans[0]
	if got.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want %v", got.Status.Code, codes.Error)
	}
	if got.Status.Description != "boom" {
		t.Fatalf("span status description = %q, want boom", got.Status.Description)
	}
	if len(got.Events) != 1 {
		t.Fatalf("span recorded %d events, want 1", len(got.Events))
	}
	if got.Events[0].Name != "exception" {
		t.Fatalf("error event name = %q, want exception", got.Events[0].Name)
	}
}

func TestSpanAttributesAndEvents(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))

	_, span := observer.StartSpan(context.Background(), "operation", glk.BoolAttr("start", true))
	span.SetAttributes(glk.FloatAttr("duration", 1.5))
	span.AddEvent("cache.hit", glk.StringAttr("key", "user:1"))
	span.SetStatus(glk.SpanStatusOK, "done")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported %d spans, want 1", len(spans))
	}
	got := spans[0]
	if got.Status.Code != codes.Ok {
		t.Fatalf("span status = %v, want %v", got.Status.Code, codes.Ok)
	}
	if attr := attributeValue(got, "start"); attr != true {
		t.Fatalf("start = %v, want true", attr)
	}
	if attr := attributeValue(got, "duration"); attr != 1.5 {
		t.Fatalf("duration = %v, want 1.5", attr)
	}
	if len(got.Events) != 1 {
		t.Fatalf("span recorded %d events, want 1", len(got.Events))
	}
	if got.Events[0].Name != "cache.hit" {
		t.Fatalf("event name = %q, want cache.hit", got.Events[0].Name)
	}
	if eventAttr := got.Events[0].Attributes[0]; string(eventAttr.Key) != "key" || eventAttr.Value.AsString() != "user:1" {
		t.Fatalf("event attribute = %v, want key=user:1", eventAttr)
	}
}

func attributeValue(span tracetest.SpanStub, key string) any {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsInterface()
		}
	}
	return nil
}
