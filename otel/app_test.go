package otel

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if !hasExceptionEvent(spans[0], "panic: boom") {
		t.Fatalf("panic span events = %+v, want exception event with panic value", spans[0].Events)
	}
}

func TestAppObservabilityRecordsHandledServerErrorDetails(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	app.GET("/fail", glk.HandlerFunc(func(ctx *glk.Context) error {
		return glk.ErrInternal("db failed", errors.New("connection refused"))
	}))

	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fail", nil))

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error || !strings.Contains(spans[0].Status.Description, "connection refused") {
		t.Fatalf("span status = %+v, want error with cause", spans[0].Status)
	}
	if !hasExceptionEvent(spans[0], "connection refused") {
		t.Fatalf("span events = %+v, want exception event", spans[0].Events)
	}
}

func hasExceptionEvent(span tracetest.SpanStub, contains string) bool {
	for _, event := range span.Events {
		if event.Name != "exception" {
			continue
		}
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.Emit(), contains) {
				return true
			}
		}
	}
	return false
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

// Unmatched requests have no route: the router's internal "/" fallback must
// not show up as http.route, and unknown methods are reported as _OTHER.
func TestAppObservabilityUnmatchedRequest(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	app.GET("/known", glk.HandlerFunc(func(ctx *glk.Context) error { return nil }))

	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("FOO1", "/missing", nil))

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "HTTP" {
		t.Fatalf("span name = %q, want HTTP", spans[0].Name)
	}
	for _, attr := range spans[0].Attributes {
		if attr.Key == "http.route" {
			t.Fatalf("unmatched request has http.route = %q", attr.Value.Emit())
		}
	}
	assertSpanAttr(t, spans[0].Attributes, "http.request.method", "_OTHER")
	assertSpanAttr(t, spans[0].Attributes, "http.request.method_original", "FOO1")
	assertSpanAttr(t, spans[0].Attributes, "url.scheme", "http")
	assertSpanAttr(t, spans[0].Attributes, "http.response.status_code", int64(http.StatusNotFound))
}

// A failure after the response started is still an error span, although the
// status sent was 200.
func TestAppObservabilityErrorAfterResponseStarted(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	app.GET("/stream", glk.HandlerFunc(func(ctx *glk.Context) error {
		ctx.ResponseWriter().Write([]byte("partial"))
		return glk.ErrInternal("stream broke", errors.New("upstream reset"))
	}))

	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/stream", nil))

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("span status = %+v, want error", spans[0].Status)
	}
	assertSpanAttr(t, spans[0].Attributes, "http.response.status_code", int64(http.StatusOK))
	assertSpanAttr(t, spans[0].Attributes, "error.type", "500")
}
