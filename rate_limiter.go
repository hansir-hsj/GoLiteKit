package golite

import (
	"context"
	"errors"
	"github/hsj/golite/logger"
	"net/http"

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

func (r *RateLimiter) RateLimiterAsMiddleware() Middleware {
	return func(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
		if !r.limiter.Allow() {
			logger.AddInfo(ctx, "rate_limited", 1)
			return ErrRateLimited
		}
		return queue.Next(ctx, w, req)
	}
}
