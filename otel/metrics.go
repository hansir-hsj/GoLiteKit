package otel

import (
	"context"
	"slices"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
)

// httpDurationBuckets are the semantic-convention bucket boundaries for
// http.server.request.duration, in seconds.
var httpDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

type metricRecorder struct {
	httpRequests       metric.Int64Counter
	httpDuration       metric.Float64Histogram // http.server.request.duration, seconds
	serviceSpanCalls   metric.Int64Counter
	serviceSpanLatency metric.Float64Histogram // seconds

	// Deprecated millisecond histograms, kept for existing dashboards.
	httpDurationMs        metric.Float64Histogram
	serviceSpanLatencyMs  metric.Float64Histogram
	allowedMetricLabels   map[string]struct{}
	hasAllowedMetricLabel bool
}

func newMetricRecorder(provider metric.MeterProvider, options Options) *metricRecorder {
	meter := provider.Meter(instrumentationName, metric.WithInstrumentationVersion(glk.Version))

	allowed := make(map[string]struct{}, len(options.MetricAttributeLabels))
	for _, key := range options.MetricAttributeLabels {
		if !metricLabelDenied(key) {
			allowed[key] = struct{}{}
		}
	}

	return &metricRecorder{
		httpRequests: int64Counter(meter, "glk.http.server.requests",
			metric.WithDescription("Number of HTTP requests served."), metric.WithUnit("{request}")),
		httpDuration: float64Histogram(meter, "http.server.request.duration",
			metric.WithDescription("Duration of HTTP server requests."), metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(httpDurationBuckets...)),
		serviceSpanCalls: int64Counter(meter, "glk.service.span.calls",
			metric.WithDescription("Number of completed service spans."), metric.WithUnit("{call}")),
		serviceSpanLatency: float64Histogram(meter, "glk.service.span.duration",
			metric.WithDescription("Duration of service spans."), metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(httpDurationBuckets...)),
		httpDurationMs: float64Histogram(meter, "glk.http.server.duration_ms",
			metric.WithDescription("Deprecated: use http.server.request.duration."), metric.WithUnit("ms")),
		serviceSpanLatencyMs: float64Histogram(meter, "glk.service.span.duration_ms",
			metric.WithDescription("Deprecated: use glk.service.span.duration."), metric.WithUnit("ms")),
		allowedMetricLabels:   allowed,
		hasAllowedMetricLabel: len(allowed) > 0,
	}
}

// int64Counter and float64Histogram report creation errors to the global
// OpenTelemetry error handler and fall back to no-op instruments, so a
// misbehaving provider can never leave a nil instrument behind.
func int64Counter(m metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	c, err := m.Int64Counter(name, opts...)
	if err != nil {
		otelapi.Handle(err)
	}
	if err != nil || c == nil {
		return noopmetric.Int64Counter{}
	}
	return c
}

func float64Histogram(m metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	h, err := m.Float64Histogram(name, opts...)
	if err != nil {
		otelapi.Handle(err)
	}
	if err != nil || h == nil {
		return noopmetric.Float64Histogram{}
	}
	return h
}

// httpRequestAttrs are the semconv attributes of a server request metric.
// method must already be normalized (see normalizeMethod); route and
// errorType are omitted when empty.
type httpRequestAttrs struct {
	method    string
	route     string
	scheme    string
	status    int
	errorType string
}

func (r *metricRecorder) recordHTTP(ctx context.Context, a httpRequestAttrs, elapsed time.Duration) {
	if r == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", a.method),
		attribute.Int("http.response.status_code", a.status),
		attribute.String("url.scheme", a.scheme),
	}
	if a.route != "" {
		attrs = append(attrs, attribute.String("http.route", a.route))
	}
	if a.errorType != "" {
		attrs = append(attrs, attribute.String("error.type", a.errorType))
	}
	set := metric.WithAttributeSet(attribute.NewSet(attrs...))
	r.httpRequests.Add(ctx, 1, set)
	r.httpDuration.Record(ctx, elapsed.Seconds(), set)
	r.httpDurationMs.Record(ctx, float64(elapsed)/float64(time.Millisecond), set)
}

func (r *metricRecorder) recordServiceSpan(ctx context.Context, name string, status glk.SpanStatus, elapsed time.Duration, attrs []glk.Attribute) {
	if r == nil {
		return
	}
	metricAttrs := []attribute.KeyValue{
		attribute.String("span.name", name),
		attribute.String("span.status", spanStatusLabel(status)),
	}
	metricAttrs = append(metricAttrs, r.metricAttributes(attrs)...)
	set := metric.WithAttributeSet(attribute.NewSet(metricAttrs...))
	r.serviceSpanCalls.Add(ctx, 1, set)
	r.serviceSpanLatency.Record(ctx, elapsed.Seconds(), set)
	r.serviceSpanLatencyMs.Record(ctx, float64(elapsed)/float64(time.Millisecond), set)
}

// metricAttributes keeps the span attributes whose keys are allowlisted with
// WithMetricAttributeLabels. The allowlist is built once in newMetricRecorder.
func (r *metricRecorder) metricAttributes(attrs []glk.Attribute) []attribute.KeyValue {
	if !r.hasAllowedMetricLabel || len(attrs) == 0 {
		return nil
	}
	var mapped []attribute.KeyValue
	for _, attr := range attrs {
		if _, ok := r.allowedMetricLabels[attr.Key]; !ok {
			continue
		}
		mapped = append(mapped, mapAttributes([]glk.Attribute{attr})...)
	}
	return mapped
}

// deniedMetricLabels are high-cardinality keys never used as metric labels,
// even when allowlisted.
var deniedMetricLabels = []string{
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

func metricLabelDenied(key string) bool {
	return slices.Contains(deniedMetricLabels, key)
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
