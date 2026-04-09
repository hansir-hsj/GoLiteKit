package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"os"

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

			tw := newTimeoutResponseWriter(w)

			doneChan := make(chan struct{})
			panicChan := make(chan any, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
					}
					close(doneChan)
				}()

				next.ServeHTTP(tw, r.WithContext(ctx))
			}()

			select {
			case p := <-panicChan:
				panic(p)
			case <-ctx.Done():
				tw.markTimeout()
				cause := context.Cause(ctx)
				SetError(ctx, ErrTimeout(fmt.Sprintf("Request timeout: %v", cause)))
				// Do not wait for the handler goroutine: it may block on I/O that
				// ignores context cancellation. Start a background drainer to catch
				// any late panic and log it, then let the goroutine finish on its own.
				go func() {
					select {
					case p := <-panicChan:
						fmt.Fprintf(os.Stderr, "panic in timed-out handler: %v\n", p)
					case <-doneChan:
					}
				}()
			case <-doneChan:
				// doneChan is closed inside defer, after an optional panic write, so check panicChan first.
				select {
				case p := <-panicChan:
					panic(p)
				default:
				}
			}
		})
	}
}
