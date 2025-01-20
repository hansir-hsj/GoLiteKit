package golite

import (
	"context"
	"log"
	"net/http"
	"time"
)

func TimeoutMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
