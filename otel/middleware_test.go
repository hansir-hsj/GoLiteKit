package otel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"github.com/hansir-hsj/GoLiteKit/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	writeDeadline time.Time
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.writeDeadline = deadline
	return nil
}

func TestMiddlewareCreatesRequestSpanAndInjectsLogFields(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))
	middleware := Middleware(observer)

	ctx := logger.WithLoggerContext(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/users/123?active=true", nil).WithContext(ctx)
	req.Pattern = "GET /users/{id}"
	recorder := httptest.NewRecorder()

	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if glk.ObserverFromContext(ctx) != observer {
			t.Fatalf("observer was not attached to handler context")
		}
		w.WriteHeader(http.StatusCreated)
		return nil
	})

	if err := handler(ctx, recorder, req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Name != "HTTP GET /users/{id}" {
		t.Fatalf("span name = %q, want %q", span.Name, "HTTP GET /users/{id}")
	}
	if span.Status.Code != codes.Ok {
		t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Ok)
	}
	assertSpanAttr(t, span.Attributes, string(semconv.HTTPRequestMethodKey), http.MethodGet)
	assertSpanAttr(t, span.Attributes, string(semconv.HTTPRouteKey), "/users/{id}")
	assertSpanAttr(t, span.Attributes, string(semconv.HTTPResponseStatusCodeKey), int64(http.StatusCreated))
	assertHasPositiveDuration(t, span.Attributes)

	fields := loggerFields(ctx)
	traceID, ok := fields["trace_id"].(string)
	if !ok || traceID == "" {
		t.Fatalf("trace_id log field missing or empty: %#v", fields["trace_id"])
	}
	spanID, ok := fields["span_id"].(string)
	if !ok || spanID == "" {
		t.Fatalf("span_id log field missing or empty: %#v", fields["span_id"])
	}
}

func TestMiddlewareRecordsHTTPMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	observer := NewObserver(
		WithTracerProvider(tracerProvider),
		WithMeterProvider(meterProvider),
	)
	middleware := Middleware(observer)

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	req.Pattern = "GET /users/{id}"
	recorder := httptest.NewRecorder()
	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusAccepted)
		return nil
	})

	if err := handler(context.Background(), recorder, req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	assertMetricExists(t, rm, "glk.http.server.requests")
	assertMetricExists(t, rm, "glk.http.server.duration_ms")
	assertMetricMissing(t, rm, "glk.service.span.calls")
	assertMetricMissing(t, rm, "glk.service.span.duration_ms")
}

func TestMiddlewareMarksServerError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))
	middleware := Middleware(observer)

	ctx := context.Background()
	req := httptest.NewRequest(http.MethodPost, "/boom", nil)
	recorder := httptest.NewRecorder()
	wantErr := errors.New("boom")

	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusInternalServerError)
		return wantErr
	})

	if err := handler(ctx, recorder, req); !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v, want %v", err, wantErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Error)
	}
	assertSpanAttr(t, span.Attributes, string(semconv.HTTPResponseStatusCodeKey), int64(http.StatusInternalServerError))
	assertSpanAttr(t, span.Attributes, string(semconv.ErrorTypeKey), "500")
	if len(span.Events) == 0 {
		t.Fatalf("expected recorded error event")
	}
}

// Without an error handler inside, a panic passes through the middleware but
// is still recorded.
func TestMiddlewareRecordsPanicAndRepanics(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	observer := NewObserver(WithTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))))
	handler := Middleware(observer)(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		panic("boom")
	})

	func() {
		defer func() {
			if p := recover(); p != "boom" {
				t.Fatalf("recovered %v, want the original panic", p)
			}
		}()
		handler(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("spans = %+v, want one error span", spans)
	}
	assertSpanAttr(t, spans[0].Attributes, string(semconv.HTTPResponseStatusCodeKey), int64(http.StatusInternalServerError))
}

func TestMiddlewareJoinsIncomingTraceAsServerSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))
	middleware := Middleware(observer, WithPropagator(propagation.TraceContext{}))

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return nil
	})
	if err := handler(context.Background(), httptest.NewRecorder(), req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != traceID {
		t.Fatalf("trace id = %s, want %s", got, traceID)
	}
	if !spans[0].Parent.IsRemote() {
		t.Fatal("expected remote parent span context")
	}
	if spans[0].SpanKind != trace.SpanKindServer {
		t.Fatalf("span kind = %v, want server", spans[0].SpanKind)
	}
}

func TestSpanEndIsIdempotent(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	observer := NewObserver(WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))))
	_, span := observer.StartSpan(context.Background(), "op")
	span.End()
	span.End()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "glk.service.span.calls" {
				continue
			}
			sum := m.Data.(metricdata.Sum[int64])
			if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
				t.Fatalf("span calls = %+v, want a single count of 1", sum.DataPoints)
			}
			return
		}
	}
	t.Fatal("glk.service.span.calls not recorded")
}

func TestMiddlewareRecordsFinalStatusAfterInformational(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	middleware := Middleware(NewObserver(WithTracerProvider(provider)))

	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Link", "</a.css>; rel=preload")
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusCreated)
		return nil
	})
	if err := handler(context.Background(), httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	assertSpanAttr(t, spans[0].Attributes, string(semconv.HTTPResponseStatusCodeKey), int64(http.StatusCreated))
}

func TestRoutePattern(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		pattern string
		want    string
	}{
		{name: "empty without mux pattern", method: http.MethodGet, path: "/users/123", want: ""},
		{name: "trims method prefix", method: http.MethodGet, path: "/users/123", pattern: "GET /users/{id}", want: "/users/{id}"},
		{name: "keeps non-prefixed pattern", method: http.MethodGet, path: "/users/123", pattern: "/users/{id}", want: "/users/{id}"},
		{name: "HEAD matched by GET pattern", method: http.MethodHead, path: "/users/123", pattern: "GET /users/{id}", want: "/users/{id}"},
		{name: "strips host", method: http.MethodGet, path: "/users/123", pattern: "GET example.com/users/{id}", want: "/users/{id}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Pattern = tt.pattern
			if got := routePattern(req); got != tt.want {
				t.Fatalf("routePattern() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMiddlewareKeepsFirstWriteHeaderStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	observer := NewObserver(WithTracerProvider(provider))
	middleware := Middleware(observer)

	req := httptest.NewRequest(http.MethodGet, "/double-write", nil)
	recorder := httptest.NewRecorder()
	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusInternalServerError)
		w.WriteHeader(http.StatusOK)
		return nil
	})

	if err := handler(context.Background(), recorder, req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Error)
	}
	assertSpanAttr(t, span.Attributes, string(semconv.HTTPResponseStatusCodeKey), int64(http.StatusInternalServerError))
}

func TestStatusCaptureUnwrapsForResponseController(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	capture := &statusCapture{ResponseWriter: rec, statusCode: http.StatusOK}
	deadline := time.Now().Add(time.Second)

	if err := http.NewResponseController(capture).SetWriteDeadline(deadline); err != nil {
		t.Fatalf("ResponseController.SetWriteDeadline: %v", err)
	}
	if !rec.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", rec.writeDeadline, deadline)
	}
}

func TestClientErrorAsSpanError(t *testing.T) {
	t.Run("4xx is OK by default", func(t *testing.T) {
		span := runStatusMiddleware(t, http.StatusNotFound)
		if span.Status.Code != codes.Ok {
			t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Ok)
		}
	})

	t.Run("4xx is error when enabled", func(t *testing.T) {
		span := runStatusMiddleware(t, http.StatusNotFound, WithClientErrorAsSpanError(true))
		if span.Status.Code != codes.Error {
			t.Fatalf("span status = %v, want %v", span.Status.Code, codes.Error)
		}
	})
}

func runStatusMiddleware(t *testing.T, status int, opts ...Option) tracetest.SpanStub {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	opts = append(opts, WithTracerProvider(provider))
	observer := NewObserver(opts...)
	middleware := Middleware(observer, opts...)

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	recorder := httptest.NewRecorder()
	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(status)
		return nil
	})

	if err := handler(context.Background(), recorder, req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	return spans[0]
}

func loggerFields(ctx context.Context) map[string]any {
	fields := map[string]any{}
	logCtx := logger.GetLoggerContext(ctx)
	if logCtx == nil {
		return fields
	}
	for node := logCtx.Head; node != nil; node = node.Next {
		fields[node.Key] = node.Value
	}
	return fields
}

func assertSpanAttr(t *testing.T, attrs []attribute.KeyValue, key string, want any) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) != key {
			continue
		}
		got := attr.Value.AsInterface()
		if got != want {
			t.Fatalf("attribute %s = %#v, want %#v", key, got, want)
		}
		return
	}
	t.Fatalf("attribute %s not found", key)
}

func assertHasPositiveDuration(t *testing.T, attrs []attribute.KeyValue) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) != "http.server.duration_ms" {
			continue
		}
		got, ok := attr.Value.AsInterface().(float64)
		if !ok {
			t.Fatalf("http.server.duration_ms = %#v, want float64", attr.Value.AsInterface())
		}
		if got < 0 {
			t.Fatalf("http.server.duration_ms = %v, want non-negative", got)
		}
		return
	}
	t.Fatalf("attribute http.server.duration_ms not found")
}
