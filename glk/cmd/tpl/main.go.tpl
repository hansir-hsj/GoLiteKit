package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	kit "github.com/hansir-hsj/GoLiteKit"
	"github.com/hansir-hsj/GoLiteKit/env"

	"{{.Module}}/controller"
)

func main() {
	app, err := kit.NewAppFromConfig("conf/app.toml")
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	app.GET("/hello", &controller.HelloController{})

	config := kit.ServerConfig{
		Addr:              env.Addr(),
		Network:           env.Network(),
		ReadTimeout:       env.ReadTimeout(),
		WriteTimeout:      env.WriteTimeout(),
		IdleTimeout:       env.IdleTimeout(),
		ReadHeaderTimeout: env.ReadHeaderTimeout(),
		MaxHeaderBytes:    env.MaxHeaderBytes(),
		ShutdownTimeout:   env.ShutdownTimeout(),
	}
	if env.TLS() {
		config.TLSCertFile = env.TLSCertFile()
		config.TLSKeyFile = env.TLSKeyFile()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.ListenAndServe(ctx, config); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
