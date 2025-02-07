package golite

import (
	"context"
	"net/http"
)

type StaticController struct {
	BaseController
	Path string
}

func (c *StaticController) Serve(ctx context.Context) error {
	http.ServeFile(c.responseWriter, c.request, c.Path)
	return nil
}
