package golitekit

import (
	"context"
	"net/http"
)

// ControllerWrapper wraps a Handler to conform to the Controller interface.
type ControllerWrapper struct {
	BaseController
	handler Handler
}

func (c *ControllerWrapper) Init(ctx context.Context) error {
	return c.BaseController.Init(ctx)
}

func (c *ControllerWrapper) Serve(ctx context.Context) error {
	gcx := GetContext(ctx)
	return c.handler(ctx, gcx.responseWriter, gcx.request)
}

func (c *ControllerWrapper) Finalize(ctx context.Context) error {
	return c.BaseController.Finalize(ctx)
}

// WrapFunc wraps a Handler into a Controller. Errors are propagated normally.
func WrapFunc(f Handler) Controller {
	return &ControllerWrapper{handler: f}
}

// WrapHandler wraps an http.Handler into a Controller.
// Note: http.Handler has no error return, so errors cannot propagate.
// Use WrapFunc with a Handler for full error propagation.
func WrapHandler(handler http.Handler) Controller {
	return &ControllerWrapper{
		handler: func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handler.ServeHTTP(w, r)
			return nil
		},
	}
}
