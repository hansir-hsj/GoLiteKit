package golitekit

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/env"
)

func TimeoutMiddleware() HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			timeout := env.WriteTimeout()
			if timeout < 1 {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("request timeout after %v", timeout))
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
						if err := ctx.Err(); err != nil {
							if err != context.Canceled {
								return
							}
						}
						panicChan <- p
					}
				}()

				select {
				case <-ctx.Done():
					return
				default:
					next.ServeHTTP(w, r)
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
			case <-ctx.Done():
				cause := context.Cause(ctx)
				log.Printf("request canceled: %v", cause)
			case <-doneChan:
				return
			}
		})
	}
}
