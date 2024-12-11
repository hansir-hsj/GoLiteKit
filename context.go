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
	gbc := ctx.Value(globalContextKey)
	if c, ok := gbc.(*Context); ok {
		return c
	}
	return nil
}

func WithContext(ctx context.Context) context.Context {
	gbc := GetContext(ctx)
	if gbc == nil {
		gbc = &Context{}
		return context.WithValue(ctx, globalContextKey, gbc)
	}
	return ctx
}
