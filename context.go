package golite

import (
	"context"
	"net/http"
)

const (
	globalContextKey ContextKey = iota
)

type ContextKey int

type ContextOption func(*Context)

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

func (gcx *Context) SetContextOptions(opts ...ContextOption) *Context {
	for _, opt := range opts {
		opt(gcx)
	}
	return gcx
}

func WithRequest(r *http.Request) ContextOption {
	return func(gcx *Context) {
		gcx.request = r
	}
}

func WithResponseWriter(w http.ResponseWriter) ContextOption {
	return func(gcx *Context) {
		gcx.responseWriter = w
	}
}

func (ctx *Context) Request() *http.Request {
	return ctx.request
}

func (ctx *Context) ResponseWriter() http.ResponseWriter {
	return ctx.responseWriter
}
