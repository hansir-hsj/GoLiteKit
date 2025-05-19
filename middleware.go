package golitekit

import (
	"context"
	"slices"
)

type Middleware func(ctx context.Context, queue MiddlewareQueue) error

type MiddlewareQueue []Middleware

func NewMiddlewareQueue(middlewares ...Middleware) MiddlewareQueue {
	return middlewares
}

func (mq MiddlewareQueue) Clone() MiddlewareQueue {
	return slices.Clone(mq)
}

func (mq *MiddlewareQueue) Use(middlewares ...Middleware) {
	*mq = append(*mq, middlewares...)
}

func (mq *MiddlewareQueue) Next(ctx context.Context) error {
	if len(*mq) == 0 {
		return nil
	}
	handler := (*mq)[0]
	*mq = (*mq)[1:]
	return handler(ctx, *mq)
}
