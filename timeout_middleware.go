package golite

import (
	"context"
	"github/hsj/golite/env"
	"log"
	"net/http"
)

func TimeoutMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
	timeout := env.WriteTimeout() - env.ReadTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	doneChan := make(chan struct{}, 1)
	panicChan := make(chan any, 1)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				gcx := GetContext(ctx)
				gcx.PanicLogger().Report(ctx, p)
				panicChan <- p
			}
		}()

		queue.Next(ctx, w, req)

		doneChan <- struct{}{}
	}()

	select {
	case p := <-panicChan:
		log.Printf("%v", p)
	case <-ctx.Done():
		log.Print("timeout")
	case <-doneChan:
	}

	return nil
}
