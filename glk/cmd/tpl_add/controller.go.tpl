package controller

import (
	"context"
	"net/http"

	kit "github.com/hansir-hsj/GoLiteKit"
)

type {{.Name}}Controller struct {
	kit.BaseController
}

func (c *{{.Name}}Controller) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"message": "ok"})
}
