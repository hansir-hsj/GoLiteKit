package golitekit

import (
	"context"
	"net"
	"net/http"
)

// RateLimiterAsMiddleware returns a middleware that enforces rate limits using keyFunc.
func (r *RateLimiter) RateLimiterAsMiddleware(keyFunc func(r *http.Request) string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			if r.enableGlobal && r.globalLimiter != nil {
				if !r.globalLimiter.Allow() {
					return ErrTooManyRequests("Global rate limit exceeded")
				}
			}

			if keyFunc != nil {
				key := keyFunc(req)
				limiter := r.GetLimiter(key)

				if !limiter.Allow() {
					return ErrTooManyRequests("Rate limit exceeded")
				}
			}

			return next(ctx, w, req)
		}
	}
}

// ByIP returns the client IP address (without port) for use as a rate limiter key.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ByPath returns the request URL path for use as a rate limiter key.
func ByPath(r *http.Request) string {
	return r.URL.Path
}
