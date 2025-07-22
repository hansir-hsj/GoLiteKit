package core

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
				return
			}
			defer tracker.LogTracker(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
