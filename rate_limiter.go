package golitekit

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

// limiterEntry pairs a rate.Limiter with the last-access timestamp used for
// sliding-window TTL expiry.
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu            sync.Mutex
	limiters      map[string]*limiterEntry
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

// GetLimiter returns the rate.Limiter for the given key, creating one if
// necessary.  When a TTL is configured, the entry's expiry clock is reset on
// every access (sliding-window semantics): an entry is only removed after it
// has been idle for a full TTL period.
func (r *RateLimiter) GetLimiter(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.limiters[key]
	if exists {
		if r.ttl > 0 {
			entry.lastSeen = time.Now()
		}
		return entry.limiter
	}

	entry = &limiterEntry{
		limiter:  rate.NewLimiter(r.rate, r.burst),
		lastSeen: time.Now(),
	}
	r.limiters[key] = entry

	if r.ttl > 0 {
		r.scheduleExpiry(key)
	}

	return entry.limiter
}

// scheduleExpiry must be called with r.mu held.  It fires after r.ttl and
// removes the entry only if it has been idle for at least r.ttl; otherwise it
// reschedules for the remaining idle time.  This ensures entries are cleaned
// up exactly one TTL after their last access, regardless of how many times
// the entry was touched.
func (r *RateLimiter) scheduleExpiry(key string) {
	time.AfterFunc(r.ttl, func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		e, ok := r.limiters[key]
		if !ok {
			return
		}

		idle := time.Since(e.lastSeen)
		if idle >= r.ttl {
			// Entry has been idle long enough — remove it.
			delete(r.limiters, key)
			return
		}

		// Entry was accessed recently.  Reschedule for the remaining idle
		// duration so we check again after another quiet period.
		time.AfterFunc(r.ttl-idle, func() {
			r.mu.Lock()
			defer r.mu.Unlock()

			if e2, ok2 := r.limiters[key]; ok2 && time.Since(e2.lastSeen) >= r.ttl {
				delete(r.limiters, key)
			}
		})
	})
}
