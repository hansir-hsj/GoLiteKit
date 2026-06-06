package otel

import (
	"slices"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	ServiceName            string
	ClientErrorAsSpanError bool
	MetricAttributeLabels  []string
	TracerProvider         trace.TracerProvider
	MeterProvider          metric.MeterProvider
}

type Option func(*Options)

func defaultOptions() Options {
	return Options{}
}

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

func applyOptions(opts []Option) Options {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}
