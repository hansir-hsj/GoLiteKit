package otel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestAppObservabilityRecordsHandledAppErrorStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	app.GET("/bad", glk.HandlerFunc(func(ctx *glk.Context) error {
		return glk.ErrBadRequest("bad request", nil)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Ok {
		t.Fatalf("4xx span status = %v, want ok by default", spans[0].Status.Code)
	}
}

func TestAppObservabilityRecordsPanicAsServerError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	app.GET("/panic", glk.HandlerFunc(func(ctx *glk.Context) error {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("panic span status = %v, want error", spans[0].Status.Code)
	}
}

func TestAppObservabilityRecordsFourxxAsErrorWhenEnabled(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(
		WithTracerProvider(provider),
		WithClientErrorAsSpanError(true),
	))
	app.GET("/bad", glk.HandlerFunc(func(ctx *glk.Context) error {
		return glk.ErrBadRequest("bad request", nil)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	app.Handler().ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("4xx span status = %v, want error when enabled", spans[0].Status.Code)
	}
}
