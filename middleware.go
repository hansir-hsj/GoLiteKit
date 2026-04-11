package golitekit

import (
	"context"
	"net/http"
	"slices"
)

// Handler is the core handler type. The returned error propagates up the
// middleware chain and is caught by ErrorHandlerMiddleware.
type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request) error

// ServeHTTP implements http.Handler. Unhandled errors are written as a plain
// text response so Handler can be used directly with http.ServeMux in tests.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(r.Context(), w, r); err != nil {
		if appErr, ok := err.(*AppError); ok {
			http.Error(w, appErr.Message, appErr.Code)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// Middleware wraps a Handler to add cross-cutting behaviour.
type Middleware func(next Handler) Handler

// MiddlewareQueue is an ordered list of Middleware values.
type MiddlewareQueue []Middleware

// NewMiddlewareQueue returns a MiddlewareQueue from the given middlewares.
func NewMiddlewareQueue(middlewares ...Middleware) MiddlewareQueue {
	return middlewares
}

// Clone returns a shallow copy of the queue.
func (mq MiddlewareQueue) Clone() MiddlewareQueue {
	return slices.Clone(mq)
}

// Use appends middlewares to the queue.
func (mq *MiddlewareQueue) Use(middlewares ...Middleware) {
	*mq = append(*mq, middlewares...)
}

// Apply wraps handler with all middlewares, outermost first.
func (mq MiddlewareQueue) Apply(handler Handler) Handler {
	for i := len(mq) - 1; i >= 0; i-- {
		handler = mq[i](handler)
	}
	return handler
}

// StdMiddleware adapts a standard net/http middleware to Middleware.
// Use this to integrate third-party middlewares (e.g. CORS) with the framework.
func StdMiddleware(m func(http.Handler) http.Handler) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			var handlerErr error
			adapted := m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerErr = next(r.Context(), w, r)
			}))
			adapted.ServeHTTP(w, r)
			return handlerErr
		}
	}
}
