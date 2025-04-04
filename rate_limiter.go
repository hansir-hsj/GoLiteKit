package golitekit

import (
	"context"
	"errors"

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

func (r *RateLimiter) RateLimiterAsMiddleware() Middleware {
	return func(ctx context.Context, queue MiddlewareQueue) error {
		if !r.limiter.Allow() {
			logger.AddInfo(ctx, "rate_limited", 1)
			return ErrRateLimited
		}
		return queue.Next(ctx)
	}
}
