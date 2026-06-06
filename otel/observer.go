package otel

import (
	"context"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/trace"
)

type Observer struct {
	options Options
	tracer  trace.Tracer
	metrics *metricRecorder
}

func NewObserver(opts ...Option) *Observer {
	options := applyOptions(opts)
	provider := options.TracerProvider
	if provider == nil {
		provider = trace.NewNoopTracerProvider()
	}

	return &Observer{
		options: options,
		tracer:  provider.Tracer(options.ServiceName),
		metrics: newMetricRecorder(options),
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
