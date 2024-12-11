package golite

import (
	"context"
	"net/http"
)

type ContextKey int

const (
	globalContextKey ContextKey = iota
)

type Context struct {
	request        *http.Request
	responseWriter http.ResponseWriter
}

func GetContext(ctx context.Context) *Context {
	gcx := ctx.Value(globalContextKey)
	if c, ok := gcx.(*Context); ok {
		return c
	}
	return nil
}

func WithContext(ctx context.Context) context.Context {
	gcx := GetContext(ctx)
	if gcx == nil {
		gcx = &Context{}
		return context.WithValue(ctx, globalContextKey, gcx)
	}
	return ctx
}

func (ctx *Context) SetRequest(r *http.Request) {
	ctx.request = r
}

func (ctx *Context) Request() *http.Request {
	return ctx.request
}

func (ctx *Context) SetResponseWriter(w http.ResponseWriter) {
	ctx.responseWriter = w
}

func (ctx *Context) ResponseWriter() http.ResponseWriter {
	return ctx.responseWriter
}
