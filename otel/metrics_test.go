package otel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestWithMeterProviderStoresProvider(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	options := applyOptions([]Option{WithMeterProvider(provider)})
	if options.MeterProvider != provider {
		t.Fatalf("MeterProvider = %v, want %v", options.MeterProvider, provider)
	}
}

func TestMetricAttributesUseAllowlistAndDenylist(t *testing.T) {
	options := applyOptions([]Option{
		WithMetricAttributeLabels("component", "db.statement", "trace_id", "user.id"),
	})
	recorder := newMetricRecorder(noopmetric.NewMeterProvider(), options)

	attrs := recorder.metricAttributes([]glk.Attribute{
		glk.StringAttr("component", "cache"),
		glk.StringAttr("db.statement", "select * from users"),
		glk.StringAttr("trace_id", "abc"),
		glk.StringAttr("user.id", "42"),
		glk.StringAttr("ignored", "value"),
	})

	if len(attrs) != 1 {
		t.Fatalf("metric attrs = %v, want exactly component", attrs)
	}
	if string(attrs[0].Key) != "component" || attrs[0].Value.AsString() != "cache" {
		t.Fatalf("metric attr = %v, want component=cache", attrs[0])
	}
}

func TestObserverRecordsServiceSpanMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	observer := NewObserver(
		WithMeterProvider(provider),
		WithMetricAttributeLabels("component"),
	)

	_, span := observer.StartSpan(context.Background(), "cache.lookup", glk.StringAttr("component", "cache"))
	span.SetStatus(glk.SpanStatusOK, "done")
	span.End()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	assertMetricExists(t, rm, "glk.service.span.calls")
	assertMetricExists(t, rm, "glk.service.span.duration")
	assertMetricExists(t, rm, "glk.service.span.duration_ms") // deprecated, still recorded
}

// Instruments use the instrumentation scope (not the service name), seconds
// as the duration unit, and semantic-convention names.
func TestMetricsScopeAndUnits(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	observer := NewObserver(WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))))
	observer.metrics.recordHTTP(context.Background(), httpRequestAttrs{method: "GET", route: "/x", scheme: "http", status: 200}, 1500*time.Millisecond)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if len(rm.ScopeMetrics) != 1 {
		t.Fatalf("scopes = %d", len(rm.ScopeMetrics))
	}
	scope := rm.ScopeMetrics[0]
	if scope.Scope.Name != instrumentationName || scope.Scope.Version != glk.Version {
		t.Fatalf("scope = %+v, want %s %s", scope.Scope, instrumentationName, glk.Version)
	}
	for _, m := range scope.Metrics {
		if m.Name != "http.server.request.duration" {
			continue
		}
		if m.Unit != "s" {
			t.Fatalf("unit = %q, want s", m.Unit)
		}
		h := m.Data.(metricdata.Histogram[float64])
		if got := h.DataPoints[0].Sum; got != 1.5 {
			t.Fatalf("recorded %v, want 1.5 seconds", got)
		}
		return
	}
	t.Fatal("http.server.request.duration not recorded")
}

// A provider that fails to create instruments must not leave nil instruments.
type failingMeterProvider struct{ noopmetric.MeterProvider }
type failingMeter struct{ noopmetric.Meter }

func (failingMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter { return failingMeter{} }
func (failingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("boom")
}
func (failingMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, errors.New("boom")
}

func TestFailingInstrumentsFallBackToNoop(t *testing.T) {
	var handled atomic.Int32
	prev := otelapi.GetErrorHandler()
	otelapi.SetErrorHandler(otelapi.ErrorHandlerFunc(func(error) { handled.Add(1) }))
	defer otelapi.SetErrorHandler(prev)

	observer := NewObserver(WithMeterProvider(failingMeterProvider{}))
	observer.metrics.recordHTTP(context.Background(), httpRequestAttrs{method: "GET", route: "/x", scheme: "http", status: 200}, time.Millisecond) // must not panic
	if handled.Load() == 0 {
		t.Fatal("instrument creation errors should be reported to the otel error handler")
	}
}

func assertMetricExists(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return
			}
		}
	}
	t.Fatalf("metric %q not found in %#v", name, rm)
}

func assertMetricMissing(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				t.Fatalf("metric %q should not be recorded in %#v", name, rm)
			}
		}
	}
}
