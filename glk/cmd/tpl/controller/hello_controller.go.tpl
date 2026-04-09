package controller

import (
	"context"

	kit "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
	kit.BaseController[kit.NoBody]
}

func (c *HelloController) Serve(ctx context.Context) error {
	return c.ServeJSON(map[string]string{"message": "Hello, GoLiteKit!"})
}
