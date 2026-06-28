// hello demonstrates the minimal GoLiteKit setup: one GET endpoint.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	glk "github.com/hansir-hsj/GoLiteKit"
)

type HelloController struct {
	glk.BaseController
}

func (c *HelloController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"message": "hello, world"})
}

func main() {
	app := glk.NewApp()
	app.GET("/hello", &HelloController{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	log.Println("listening on :8080")
	if err := app.ListenAndServe(ctx, glk.ServerConfig{Addr: ":8080"}); err != nil {
		log.Fatal(err)
	}
}
