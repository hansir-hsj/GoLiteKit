package otel

import glk "github.com/hansir-hsj/GoLiteKit"

func WithObservability(opts ...Option) glk.ServiceOption {
	observer := NewObserver(opts...)
	middleware := Middleware(observer, opts...)

	return func(s *glk.Services) {
		glk.WithObserver(observer)(s)
		glk.WithObservabilityMiddleware(middleware)(s)
	}
}
