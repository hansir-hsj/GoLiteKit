package golitekit

import (
	"sync"
	"sync/atomic"
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

type limiterEntry struct {
	limiter  *rate.Limiter
	lastUsed atomic.Int64
}

type RateLimiter struct {
	mu            sync.RWMutex
	limiters      map[string]*limiterEntry
	globalLimiter *rate.Limiter
	rate          rate.Limit
	burst         int
	ttl           time.Duration
	enableGlobal  bool
	cleanCounter  atomic.Int64
}

func NewRateLimiter(rat rate.Limit, burst int, opts ...RateLimiterOption) *RateLimiter {
	options := RateLimiterOptions{}

	for _, opt := range opts {
		opt(&options)
	}

	r := &RateLimiter{
		limiters:     make(map[string]*limiterEntry),
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
	entry, exists := r.limiters[key]
	if exists {
		entry.lastUsed.Store(time.Now().UnixNano())
		r.mu.RUnlock()

		if r.ttl > 0 && r.cleanCounter.Add(1)%1000 == 0 {
			go r.cleanExpired()
		}
		return entry.limiter
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists = r.limiters[key]
	if exists {
		entry.lastUsed.Store(time.Now().UnixNano())
		return entry.limiter
	}

	entry = &limiterEntry{
		limiter: rate.NewLimiter(r.rate, r.burst),
	}
	entry.lastUsed.Store(time.Now().UnixNano())
	r.limiters[key] = entry

	return entry.limiter
}

func (r *RateLimiter) cleanExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano()
	ttlNanos := r.ttl.Nanoseconds()

	for key, entry := range r.limiters {
		if now-entry.lastUsed.Load() > ttlNanos {
			delete(r.limiters, key)
		}
	}
}
