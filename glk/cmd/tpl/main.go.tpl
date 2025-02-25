package main

import (
	kit "github.com/hansir-hsj/GoLiteKit"

	"{{.App}}/controller"
)

func main() {
	server := kit.New("conf/app.toml")
	server.OnGet("/hello", &controller.HelloController{})
	server.Start()
}
