package middleware

import (
	"context"
	"net/http"

	kit "github.com/hansir-hsj/GoLiteKit"
)

// {{.Name}}Middleware is a custom GoLiteKit middleware.
func {{.Name}}Middleware(next kit.Handler) kit.Handler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return next(ctx, w, r)
	}
}
