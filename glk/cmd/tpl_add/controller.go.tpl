package controller

import (
	"context"

	kit "github.com/hansir-hsj/GoLiteKit"
)

type {{.Name}}Controller struct {
	kit.BaseController[kit.NoBody]
}

func (c *{{.Name}}Controller) Serve(ctx context.Context) error {
	return c.ServeJSON(map[string]string{"message": "ok"})
}
