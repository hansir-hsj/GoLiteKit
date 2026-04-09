package golitekit

import (
	"net"
	"net/http"
)

// RateLimiterAsMiddleware returns a middleware that enforces rate limits using keyFunc.
func (r *RateLimiter) RateLimiterAsMiddleware(keyFunc func(r *http.Request) string) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if r.enableGlobal && r.globalLimiter != nil {
				if !r.globalLimiter.Allow() {
					SetError(req.Context(), ErrTooManyRequests("Global rate limit exceeded"))
					return
				}
			}

			if keyFunc != nil {
				key := keyFunc(req)
				limiter := r.GetLimiter(key)

				if !limiter.Allow() {
					SetError(req.Context(), ErrTooManyRequests("Rate limit exceeded"))
					return
				}
			}

			next.ServeHTTP(w, req)
		})
	}
}

// ByIP returns the client IP address (without port) for use as a rate limiter key.
func ByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr may be a bare IP in test or proxy environments.
		return r.RemoteAddr
	}
	return host
}

// ByPath returns the request URL path for use as a rate limiter key.
func ByPath(r *http.Request) string {
	return r.URL.Path
}
