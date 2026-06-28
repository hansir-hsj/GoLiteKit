package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// TimeoutOptions configures the timeout middleware.
type TimeoutOptions struct {
	Duration   time.Duration
	SSETimeout time.Duration
}

// TimeoutMiddleware creates a timeout middleware.
// Without options, no timeout is applied.
func TimeoutMiddleware(opts ...TimeoutOptions) Middleware {
	var opt TimeoutOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			timeout := opt.Duration
			sseTimeout := opt.SSETimeout

			if sseTimeout > 0 && r.Header.Get("Accept") == "text/event-stream" {
				timeout = sseTimeout
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
				return ErrTimeout(fmt.Sprintf("Request timeout: %v", context.Cause(timeoutCtx)), nil)
			}

			return err
		}
	}
}
