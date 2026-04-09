package main

import (
	"log"

	kit "github.com/hansir-hsj/GoLiteKit"

	"{{.App}}/controller"
)

func main() {
	app, err := kit.NewAppFromConfig("conf/app.toml")
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	app.GET("/hello", &controller.HelloController{})

	if err := app.RunFromEnv(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
