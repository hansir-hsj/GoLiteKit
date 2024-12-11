package datong

import (
	"context"
	"net/http"
)

type Context struct {
	ctx            context.Context
	request        *http.Request
	responseWriter http.ResponseWriter
}

func NewContext(r *http.Request, w http.ResponseWriter) *Context {
	return &Context{
		ctx:            r.Context(),
		request:        r,
		responseWriter: w,
	}
}
