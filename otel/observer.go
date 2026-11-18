package otel

import (
	"context"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// instrumentationName is the instrumentation scope of the tracer and meter.
// The service name belongs in the provider's Resource (service.name).
const instrumentationName = "github.com/hansir-hsj/GoLiteKit/otel"

type Observer struct {
	options Options
	tracer  trace.Tracer
	metrics *metricRecorder
}

// NewObserver creates an Observer. Without WithTracerProvider /
// WithMeterProvider it uses the global providers (otel.GetTracerProvider,
// otel.GetMeterProvider), which are no-ops until the application sets them.
func NewObserver(opts ...Option) *Observer {
	options := applyOptions(opts)
	tracerProvider := options.TracerProvider
	if tracerProvider == nil {
		tracerProvider = otelapi.GetTracerProvider()
	}
	meterProvider := options.MeterProvider
	if meterProvider == nil {
		meterProvider = otelapi.GetMeterProvider()
	}

	return &Observer{
		options: options,
		tracer:  tracerProvider.Tracer(instrumentationName, trace.WithInstrumentationVersion(glk.Version)),
		metrics: newMetricRecorder(meterProvider, options),
	}
}

func (o *Observer) StartSpan(ctx context.Context, name string, attrs ...glk.Attribute) (context.Context, glk.Span) {
	ctx, span := o.tracer.Start(ctx, name, trace.WithAttributes(mapAttributes(attrs)...))
	return ctx, &spanImpl{
		ctx:            ctx,
		span:           span,
		metrics:        o.metrics,
		name:           name,
		started:        time.Now(),
		attrs:          append([]glk.Attribute(nil), attrs...),
		serviceMetrics: true,
	}
}
