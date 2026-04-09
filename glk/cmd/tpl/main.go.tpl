package main

import (
	"log"

	kit "github.com/hansir-hsj/GoLiteKit"

	"{{.Module}}/controller"
)

func main() {
	app, err := kit.NewAppFromConfig("conf/app.toml")
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	app.GET("/hello", &controller.HelloController{})

	if err := app.RunFromEnv(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
