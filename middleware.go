package golitekit

import (
	"net/http"
	"slices"
)

// HandlerMiddleware wraps an http.Handler to add cross-cutting behaviour.
type HandlerMiddleware func(http.Handler) http.Handler

// MiddlewareQueue is an ordered list of middlewares applied to a handler.
type MiddlewareQueue []HandlerMiddleware

// NewMiddlewareQueue returns a MiddlewareQueue from the given middlewares.
func NewMiddlewareQueue(middlewares ...HandlerMiddleware) MiddlewareQueue {
	return middlewares
}

// Clone returns a shallow copy of the queue.
func (mq MiddlewareQueue) Clone() MiddlewareQueue {
	return slices.Clone(mq)
}

// Use appends middlewares to the queue.
func (mq *MiddlewareQueue) Use(middlewares ...HandlerMiddleware) {
	*mq = append(*mq, middlewares...)
}

// Apply wraps handler with all middlewares, outermost first.
func (mq MiddlewareQueue) Apply(handler http.Handler) http.Handler {
	for i := len(mq) - 1; i >= 0; i-- {
		handler = (mq)[i](handler)
	}
	return handler
}
