package golitekit

import (
	"context"
	"net/http"
)

// 使用 wrapFunc 注册标准的 http.HandlerFunc
// s.OnGet("/hello", wrapFunc(func(w http.ResponseWriter, r *http.Request) {
// 	fmt.Fprintf(w, "Hello, World!")
// }))

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

func WrapFunc(f http.HandlerFunc) Controller {
	return &ControllerWrapper{
		handler: f,
	}
}

func WrapHandler(handler http.Handler) Controller {
	return &ControllerWrapper{
		handler: handler.ServeHTTP,
	}
}
