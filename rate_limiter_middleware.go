package golitekit

import "net/http"

func (r *RateLimiter) RateLimiterAsMiddleware(keyFunc func(r *http.Request) string) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if r.enableGlobal && r.globalLimiter != nil {
				if !r.globalLimiter.Allow() {
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
					return
				}
			}

			if keyFunc != nil {
				key := keyFunc(req)
				limiter := r.GetLimiter(key)

				if !limiter.Allow() {
					http.Error(w, "Too many requests", http.StatusTooManyRequests)
					return
				}
			}

			next.ServeHTTP(w, req)
		})
	}
}

func ByIP(r *http.Request) string {
	return r.RemoteAddr
}

func ByPath(r *http.Request) string {
	return r.URL.Path
}
