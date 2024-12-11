package datong

import (
	"context"
	"net/http"
)

const sharedContextKey = "sharedContextKey"

type sharedContext struct {
	request  http.Request
	response http.ResponseWriter
}

type Context struct {
	sharedContext
}

func GetContext(ctx context.Context) Context {
	if context, ok := ctx.Value(sharedContextKey).(Context); ok {
		return context
	}
	return Context{}
}
