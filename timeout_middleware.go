package golite

import (
	"context"
	"fmt"
	"github/hsj/golite/env"
	"log"
)

func TimeoutMiddleware(ctx context.Context, queue MiddlewareQueue) error {
	timeout := env.WriteTimeout() - env.ReadTimeout()
	if timeout < 1 {
		return queue.Next(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	doneChan := make(chan struct{}, 1)
	panicChan := make(chan any, 1)
	defer close(doneChan)
	defer close(panicChan)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				gcx := GetContext(ctx)
				gcx.PanicLogger().Report(ctx, p)
				panicChan <- p
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
			err := queue.Next(ctx)
			if err != nil {
				log.Printf("%v", err)
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		doneChan <- struct{}{}
	}()

	select {
	case p := <-panicChan:
		log.Printf("%v", p)
		return fmt.Errorf("panic: %v", p)
	case <-ctx.Done():
		log.Print("timeout")
		return fmt.Errorf("timeout")
	case <-doneChan:
		return nil
	}
}
