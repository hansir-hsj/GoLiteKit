package golitekit

import (
	"errors"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"

	"golang.org/x/time/rate"
)

var (
	ErrRateLimited = errors.New("rate limit")
)

type RateLimiter struct {
	limiter *rate.Limiter
}

func NewRateLimiter(limit, burst int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(limit), burst),
	}
}

func (r *RateLimiter) RateLimiterAsMiddleware() HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			if !r.limiter.Allow() {
				logger.AddInfo(ctx, "rate_limited", 1)
			}

			next.ServeHTTP(w, req)
		})
	}
}
