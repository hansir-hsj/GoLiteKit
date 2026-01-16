package golitekit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/env"
)

func TimeoutMiddleware() HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := env.WriteTimeout()

			if env.SSETimeout() > 0 && r.Header.Get("Accept") == "text/event-stream" {
				timeout = env.SSETimeout()
			}

			if timeout < 1 {
				next.ServeHTTP(w, r)
				return
			}

			ctx, cancel := context.WithTimeoutCause(
				r.Context(),
				timeout,
				fmt.Errorf("request timeout after %v", timeout),
			)
			defer cancel()

			// use thread-safe ResponseWriter
			tw := newTimeoutResponseWriter(w)

			doneChan := make(chan struct{}, 1)
			panicChan := make(chan any, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
					}
				}()

				next.ServeHTTP(tw, r.WithContext(ctx))
				close(doneChan)
			}()

			select {
			case p := <-panicChan:
				// panic again, let outer ErrorHandlerMiddleware handle it
				panic(p)
			case <-ctx.Done():
				tw.markTimeout()
				cause := context.Cause(ctx)
				SetError(r.Context(), ErrTimeout(fmt.Sprintf("Request timeout: %v", cause)))
			case <-doneChan:
				return
			}
		})
	}
}
