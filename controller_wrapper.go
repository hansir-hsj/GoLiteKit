package golitekit

import (
	"context"
	"net/http"
)

// ControlWrapper is used to wrap standard http.HHandlerFunc or http.HHandler files
// Make it conform to the Controller interface of the framework
//
// Usage:
//
//	s.OnGet("/hello", WrapFunc(func(w http.ResponseWriter, r *http.Request) {
//	    fmt.Fprintf(w, "Hello, World!")
//	}))
type ControllerWrapper struct {
	BaseController[NoBody]
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

// WrapFunc wraps the standard http.HandlerFunc into a Controller
func WrapFunc(f http.HandlerFunc) Controller {
	return &ControllerWrapper{
		handler: f,
	}
}

// WrapHandler Packaging the standard http.HHandler into a Controller framework
func WrapHandler(handler http.Handler) Controller {
	return &ControllerWrapper{
		handler: handler.ServeHTTP,
	}
}
