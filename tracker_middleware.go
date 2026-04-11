package golitekit

import (
	"context"
	"net/http"
)

func TrackerMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			ctx = WithContext(ctx)
			ctx = WithTracker(ctx)
			tracker := GetTracker(ctx)
			if tracker == nil {
				return next(ctx, w, r)
			}
			defer tracker.LogTracker(ctx)
			return next(ctx, w, r.WithContext(ctx))
		}
	}
}
