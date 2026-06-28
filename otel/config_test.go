package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	glk "github.com/hansir-hsj/GoLiteKit"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWithObservabilityStoresObserverAndMiddleware(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	app := glk.NewApp(WithObservability(WithTracerProvider(provider)))
	if app.Services().Observer() == nil {
		t.Fatalf("observer was not stored")
	}
	middleware := app.Services().ObservabilityMiddleware()
	if middleware == nil {
		t.Fatalf("observability middleware was not stored")
	}

	req := httptest.NewRequest(http.MethodGet, "/wired", nil)
	recorder := httptest.NewRecorder()
	handler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if glk.ObserverFromContext(ctx) == nil {
			t.Fatalf("observer was not attached by stored middleware")
		}
		return nil
	})
	if err := handler(context.Background(), recorder, req); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(exporter.GetSpans()) != 1 {
		t.Fatalf("expected stored middleware to create one span, got %d", len(exporter.GetSpans()))
	}
}
