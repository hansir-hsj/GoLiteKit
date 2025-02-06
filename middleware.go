package golite

import (
	"context"
	"net/http"
)

type Middleware func(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error

type MiddlewareQueue []Middleware

func NewMiddlewareQueue(middlewares ...Middleware) MiddlewareQueue {
	return middlewares
}

func (mq MiddlewareQueue) Clone() MiddlewareQueue {
	return append(MiddlewareQueue{}, mq...)
}

func (mq *MiddlewareQueue) Use(middlewares ...Middleware) {
	*mq = append(*mq, middlewares...)
}

func (mq *MiddlewareQueue) Next(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	if len(*mq) == 0 {
		return nil
	}
	handler := (*mq)[0]
	*mq = (*mq)[1:]
	return handler(ctx, w, req, *mq)
}
