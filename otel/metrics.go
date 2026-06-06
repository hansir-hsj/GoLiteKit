package otel

import (
	"context"
	"slices"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type metricRecorder struct {
	httpRequests       metric.Int64Counter
	httpDuration       metric.Float64Histogram
	serviceSpanCalls   metric.Int64Counter
	serviceSpanLatency metric.Float64Histogram
	options            Options
}

func newMetricRecorder(options Options) *metricRecorder {
	if options.MeterProvider == nil {
		return nil
	}
	meter := options.MeterProvider.Meter(options.ServiceName)
	httpRequests, _ := meter.Int64Counter("glk.http.server.requests")
	httpDuration, _ := meter.Float64Histogram("glk.http.server.duration_ms")
	serviceSpanCalls, _ := meter.Int64Counter("glk.service.span.calls")
	serviceSpanLatency, _ := meter.Float64Histogram("glk.service.span.duration_ms")
	return &metricRecorder{
		httpRequests:       httpRequests,
		httpDuration:       httpDuration,
		serviceSpanCalls:   serviceSpanCalls,
		serviceSpanLatency: serviceSpanLatency,
		options:            options,
	}
}

func (r *metricRecorder) recordHTTP(ctx context.Context, method, route string, status int, elapsed time.Duration) {
	if r == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", method),
		attribute.String("http.route", route),
		attribute.Int("http.response.status_code", status),
	}
	r.httpRequests.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.httpDuration.Record(ctx, float64(elapsed)/float64(time.Millisecond), metric.WithAttributes(attrs...))
}

func (r *metricRecorder) recordServiceSpan(ctx context.Context, name string, status glk.SpanStatus, elapsed time.Duration, attrs []glk.Attribute) {
	if r == nil {
		return
	}
	metricAttrs := []attribute.KeyValue{
		attribute.String("span.name", name),
		attribute.String("span.status", spanStatusLabel(status)),
	}
	metricAttrs = append(metricAttrs, metricAttributes(r.options, attrs)...)
	r.serviceSpanCalls.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
	r.serviceSpanLatency.Record(ctx, float64(elapsed)/float64(time.Millisecond), metric.WithAttributes(metricAttrs...))
}

func metricAttributes(options Options, attrs []glk.Attribute) []attribute.KeyValue {
	if len(options.MetricAttributeLabels) == 0 || len(attrs) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(options.MetricAttributeLabels))
	for _, key := range options.MetricAttributeLabels {
		if metricLabelDenied(key) {
			continue
		}
		allowed[key] = struct{}{}
	}
	mapped := make([]attribute.KeyValue, 0, len(allowed))
	for _, attr := range attrs {
		if _, ok := allowed[attr.Key]; !ok {
			continue
		}
		mapped = append(mapped, mapAttributes([]glk.Attribute{attr})...)
	}
	return mapped
}

func metricLabelDenied(key string) bool {
	denied := []string{
		"db.statement",
		"http.target",
		"http.url",
		"logid",
		"log_id",
		"span_id",
		"trace_id",
		"url.full",
		"user.id",
	}
	return slices.Contains(denied, key)
}

func spanStatusLabel(status glk.SpanStatus) string {
	switch status {
	case glk.SpanStatusOK:
		return "ok"
	case glk.SpanStatusError:
		return "error"
	default:
		return "unset"
	}
}
