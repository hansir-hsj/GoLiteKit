package otel

import (
	"slices"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	// Deprecated: ServiceName is no longer used. The tracer and meter use the
	// instrumentation scope "github.com/hansir-hsj/GoLiteKit/otel"; set the
	// service name on the provider's Resource (service.name) instead.
	ServiceName            string
	ClientErrorAsSpanError bool
	MetricAttributeLabels  []string
	TracerProvider         trace.TracerProvider
	MeterProvider          metric.MeterProvider
	// Propagator extracts the caller's trace context from request headers.
	// Nil means the global propagator (otel.GetTextMapPropagator).
	Propagator propagation.TextMapPropagator
}

type Option func(*Options)

func defaultOptions() Options {
	return Options{}
}

// Deprecated: the service name is no longer used; see Options.ServiceName.
func WithServiceName(name string) Option {
	return func(o *Options) {
		o.ServiceName = name
	}
}

func WithClientErrorAsSpanError(enabled bool) Option {
	return func(o *Options) {
		o.ClientErrorAsSpanError = enabled
	}
}

func WithMetricAttributeLabels(keys ...string) Option {
	return func(o *Options) {
		o.MetricAttributeLabels = slices.Clone(keys)
	}
}

func WithTracerProvider(provider trace.TracerProvider) Option {
	return func(o *Options) {
		o.TracerProvider = provider
	}
}

func WithMeterProvider(provider metric.MeterProvider) Option {
	return func(o *Options) {
		o.MeterProvider = provider
	}
}

// WithPropagator sets the propagator Middleware uses to extract incoming
// trace context, e.g. propagation.TraceContext{}.
func WithPropagator(propagator propagation.TextMapPropagator) Option {
	return func(o *Options) {
		o.Propagator = propagator
	}
}

func applyOptions(opts []Option) Options {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}
