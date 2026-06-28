package controller

import (
	"context"
	"net/http"

	kit "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
	kit.BaseController
}

func (c *HelloController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"message": "Hello, GoLiteKit!"})
}
