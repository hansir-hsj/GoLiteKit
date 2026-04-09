// hello demonstrates the minimal GoLiteKit setup: one GET endpoint.
package main

import (
	"context"
	"log"

	glk "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
	glk.BaseController[glk.NoBody]
}

func (c *HelloController) Serve(ctx context.Context) error {
	return c.ServeJSON(map[string]string{"message": "hello, world"})
}

func main() {
	app := glk.NewApp()
	app.GET("/hello", &HelloController{})

	log.Println("listening on :8080")
	if err := app.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
