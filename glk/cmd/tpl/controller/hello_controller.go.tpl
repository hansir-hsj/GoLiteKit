package controller

import (
	"context"
	kit "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
	kit.BaseController
}

func (c *HelloController) Serve(ctx context.Context) error {
	c.ServeJSON("{'status': 0, 'msg': 'ok'}")
	return nil
}
