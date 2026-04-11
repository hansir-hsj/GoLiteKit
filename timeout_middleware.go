package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hansir-hsj/GoLiteKit/env"
)

func TimeoutMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			timeout := env.WriteTimeout()

			if env.SSETimeout() > 0 && r.Header.Get("Accept") == "text/event-stream" {
				timeout = env.SSETimeout()
			}

			if timeout < 1 {
				return next(ctx, w, r)
			}

			timeoutCtx, cancel := context.WithTimeoutCause(
				ctx,
				timeout,
				fmt.Errorf("request timeout after %v", timeout),
			)
			defer cancel()

			tw := newTimeoutResponseWriter(w)

			type result struct {
				err   error
				panic any
			}
			resultChan := make(chan result, 1)

			go func() {
				defer func() {
					if p := recover(); p != nil {
						resultChan <- result{panic: p}
					}
				}()
				err := next(timeoutCtx, tw, r.WithContext(timeoutCtx))
				resultChan <- result{err: err}
			}()

			select {
			case res := <-resultChan:
				if res.panic != nil {
					panic(res.panic)
				}
				return res.err
			case <-timeoutCtx.Done():
				tw.markTimeout()
				cause := context.Cause(timeoutCtx)
				// Drain the goroutine in background; re-panic if it panics.
				go func() {
					if res := <-resultChan; res.panic != nil {
						fmt.Fprintf(os.Stderr, "panic in timed-out handler: %v\n", res.panic)
					}
				}()
				return ErrTimeout(fmt.Sprintf("Request timeout: %v", cause))
			}
		}
	}
}
