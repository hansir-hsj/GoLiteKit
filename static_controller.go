package golite

import (
	"context"
	"net/http"
)

type DefaultStaticController struct {
	BaseController
	Path string
}

func (c *DefaultStaticController) Serve(ctx context.Context) error {
	http.ServeFile(c.responseWriter, c.request, c.Path)
	return nil
}
