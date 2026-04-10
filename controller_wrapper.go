package golitekit

import (
	"context"
	"net/http"
)

// ControllerWrapper wraps http.HandlerFunc to conform to Controller interface.
type ControllerWrapper struct {
	BaseController
	handler http.HandlerFunc
}

func (c *ControllerWrapper) Init(ctx context.Context) error {
	return c.BaseController.Init(ctx)
}

func (c *ControllerWrapper) Serve(ctx context.Context) error {
	gcx := GetContext(ctx)
	c.handler(gcx.responseWriter, gcx.request)
	return nil
}

func (c *ControllerWrapper) Finalize(ctx context.Context) error {
	return c.BaseController.Finalize(ctx)
}

// WrapFunc wraps http.HandlerFunc into a Controller.
func WrapFunc(f http.HandlerFunc) Controller {
	return &ControllerWrapper{
		handler: f,
	}
}

// WrapHandler wraps http.Handler into a Controller.
func WrapHandler(handler http.Handler) Controller {
	return &ControllerWrapper{
		handler: handler.ServeHTTP,
	}
}
