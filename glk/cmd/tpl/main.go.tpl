package main

import (
	"log"

	kit "github.com/hansir-hsj/GoLiteKit"

	"{{.App}}/controller"
)

func main() {
	server, err := kit.New("conf/app.toml")
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	server.OnGet("/hello", &controller.HelloController{})
	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
