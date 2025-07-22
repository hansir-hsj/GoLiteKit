package core

import (
	"net/http"
	"slices"
)

type HandlerMiddleware func(http.Handler) http.Handler

type MiddlewareQueue []HandlerMiddleware

func NewMiddlewareQueue(middlewares ...HandlerMiddleware) MiddlewareQueue {
	return middlewares
}

func (mq MiddlewareQueue) Clone() MiddlewareQueue {
	return slices.Clone(mq)
}

func (mq *MiddlewareQueue) Use(middlewares ...HandlerMiddleware) {
	*mq = append(*mq, middlewares...)
}

func (mq MiddlewareQueue) Apply(handler http.Handler) http.Handler {
	for i := len(mq) - 1; i >= 0; i-- {
		handler = (mq)[i](handler)
	}
	return handler
}
