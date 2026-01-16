package golitekit

import (
	"net/http"
)

func TrackerMiddleware() HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithContext(r.Context())
			ctx = WithTracker(ctx)
			tracker := GetTracker(ctx)
			if tracker == nil {
				// if the tracker is nil, continue executing without tracking
				next.ServeHTTP(w, r)
				return
			}
			defer tracker.LogTracker(ctx)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
