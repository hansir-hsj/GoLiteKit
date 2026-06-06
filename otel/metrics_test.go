package otel

import (
	"context"
	"testing"

	glk "github.com/hansir-hsj/GoLiteKit"
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

	attrs := metricAttributes(options, []glk.Attribute{
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
	assertMetricExists(t, rm, "glk.service.span.duration_ms")
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
