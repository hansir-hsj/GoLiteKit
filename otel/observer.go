package otel

import (
	"context"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/trace"
)

type Observer struct {
	options Options
	tracer  trace.Tracer
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
	}
}

func (o *Observer) StartSpan(ctx context.Context, name string, attrs ...glk.Attribute) (context.Context, glk.Span) {
	ctx, span := o.tracer.Start(ctx, name, trace.WithAttributes(mapAttributes(attrs)...))
	return ctx, spanImpl{span: span}
}
