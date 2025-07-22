package core

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiterOptions struct {
	EnableGlobal bool
	GlobalRate   rate.Limit
	GlobalBurst  int
	TTL          time.Duration
}

type RateLimiterOption func(*RateLimiterOptions)

func WithGlobalRateLimiter(rate rate.Limit, burst int) RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.EnableGlobal = true
		opts.GlobalRate = rate
		opts.GlobalBurst = burst
	}
}

func WithTTL(ttl time.Duration) RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.TTL = ttl
	}
}

type RateLimiter struct {
	mu            sync.RWMutex
	limiters      map[string]*rate.Limiter
	globalLimiter *rate.Limiter
	rate          rate.Limit
	burst         int
	ttl           time.Duration
	enableGlobal  bool
}

func NewRateLimiter(rat rate.Limit, burst int, opts ...RateLimiterOption) *RateLimiter {
	options := RateLimiterOptions{}

	for _, opt := range opts {
		opt(&options)
	}

	r := &RateLimiter{
		limiters:     make(map[string]*rate.Limiter),
		rate:         rat,
		burst:        burst,
		ttl:          options.TTL,
		enableGlobal: options.EnableGlobal,
	}
	if options.EnableGlobal {
		r.globalLimiter = rate.NewLimiter(options.GlobalRate, options.GlobalBurst)
	}

	return r
}

func (r *RateLimiter) GetLimiter(key string) *rate.Limiter {
	r.mu.RLock()
	limiter, exists := r.limiters[key]
	defer r.mu.RUnlock()

	if !exists {
		r.mu.Lock()
		limiter, exists = r.limiters[key]
		if !exists {
			limiter = rate.NewLimiter(r.rate, r.burst)
			r.limiters[key] = limiter

			if r.ttl > 0 {
				go func(k string) {
					time.Sleep((r.ttl))
					r.mu.Lock()
					delete(r.limiters, k)
					r.mu.Unlock()
				}(key)
			}
		}
		r.mu.Unlock()
	}

	return limiter
}
