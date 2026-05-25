package golitekit

import (
	"context"
	"fmt"
	"net/http"

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

			err := next(timeoutCtx, w, r.WithContext(timeoutCtx))

			if timeoutCtx.Err() == context.DeadlineExceeded && err == nil {
				return ErrTimeout(fmt.Sprintf("Request timeout: %v", context.Cause(timeoutCtx)))
			}

			return err
		}
	}
}
