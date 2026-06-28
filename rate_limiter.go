package golitekit

import (
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	DefaultRateLimiterTTL     = 10 * time.Minute
	DefaultRateLimiterMaxKeys = 10000
)

type RateLimiterOptions struct {
	EnableGlobal bool
	GlobalRate   rate.Limit
	GlobalBurst  int
	TTL          time.Duration
	MaxKeys      int
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

func WithoutTTL() RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.TTL = -1
	}
}

func WithMaxKeys(max int) RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.MaxKeys = max
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
	maxKeys       int
	enableGlobal  bool
	cleanCounter  atomic.Int64
}

func NewRateLimiter(rat rate.Limit, burst int, opts ...RateLimiterOption) *RateLimiter {
	options := RateLimiterOptions{}

	for _, opt := range opts {
		opt(&options)
	}
	ttl := options.TTL
	if ttl == 0 {
		ttl = DefaultRateLimiterTTL
	} else if ttl < 0 {
		ttl = 0
	}
	maxKeys := options.MaxKeys
	if maxKeys <= 0 {
		maxKeys = DefaultRateLimiterMaxKeys
	}

	r := &RateLimiter{
		limiters:     make(map[string]*limiterEntry),
		rate:         rat,
		burst:        burst,
		ttl:          ttl,
		maxKeys:      maxKeys,
		enableGlobal: options.EnableGlobal,
	}
	if options.EnableGlobal {
		r.globalLimiter = rate.NewLimiter(options.GlobalRate, options.GlobalBurst)
	}

	return r
}

func (r *RateLimiter) limiterForKey(key string) (*rate.Limiter, bool) {
	now := time.Now().UnixNano()

	r.mu.RLock()
	entry, exists := r.limiters[key]
	if exists {
		entry.lastUsed.Store(now)
		r.mu.RUnlock()

		if r.ttl > 0 && r.cleanCounter.Add(1)%1000 == 0 {
			go r.cleanExpired()
		}
		return entry.limiter, true
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists = r.limiters[key]
	if exists {
		entry.lastUsed.Store(now)
		return entry.limiter, true
	}

	if len(r.limiters) >= r.maxKeys {
		if r.ttl > 0 {
			r.cleanExpiredLocked(now)
		}
		if len(r.limiters) >= r.maxKeys {
			return nil, false
		}
	}

	entry = &limiterEntry{
		limiter: rate.NewLimiter(r.rate, r.burst),
	}
	entry.lastUsed.Store(now)
	r.limiters[key] = entry

	return entry.limiter, true
}

func (r *RateLimiter) cleanExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cleanExpiredLocked(time.Now().UnixNano())
}

func (r *RateLimiter) cleanExpiredLocked(now int64) {
	if r.ttl <= 0 {
		return
	}
	ttlNanos := r.ttl.Nanoseconds()

	for key, entry := range r.limiters {
		if now-entry.lastUsed.Load() > ttlNanos {
			delete(r.limiters, key)
		}
	}
}
