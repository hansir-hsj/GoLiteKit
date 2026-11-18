package golitekit

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	DefaultRateLimiterTTL     = 10 * time.Minute
	DefaultRateLimiterMaxKeys = 10000

	// rateLimiterMaxShards bounds lock contention: keys are spread over up to
	// this many independently locked shards, one per rateLimiterKeysPerShard keys.
	rateLimiterMaxShards    = 16
	rateLimiterKeysPerShard = 512
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

// WithTTL sets how long an unused key's limiter is kept (default 10 minutes).
func WithTTL(ttl time.Duration) RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.TTL = ttl
	}
}

// WithoutTTL keeps unused keys until they are evicted for capacity.
func WithoutTTL() RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.TTL = -1
	}
}

// WithMaxKeys bounds how many keys are tracked (default 10000). When full, the
// least recently used key is evicted to make room for a new one.
func WithMaxKeys(max int) RateLimiterOption {
	return func(opts *RateLimiterOptions) {
		opts.MaxKeys = max
	}
}

type limiterEntry struct {
	key      string
	limiter  *rate.Limiter
	lastUsed int64 // unix nanoseconds
}

// limiterShard is an LRU of limiters: the list front is the most recently used.
type limiterShard struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	lru      list.List
	capacity int
}

// RateLimiter keeps a token bucket per key. At most MaxKeys are kept; the
// least recently used key is evicted, and keys idle past the TTL are dropped.
type RateLimiter struct {
	shards        []limiterShard
	globalLimiter *rate.Limiter
	rate          rate.Limit
	burst         int
	ttl           time.Duration
	maxKeys       int
	enableGlobal  bool
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

	// Shard only large limits: each shard is its own LRU, so eviction order is
	// exact with one shard and approximate (per shard) with several. Shard
	// capacities add up to exactly maxKeys.
	numShards := min(rateLimiterMaxShards, max(1, maxKeys/rateLimiterKeysPerShard))

	r := &RateLimiter{
		shards:       make([]limiterShard, numShards),
		rate:         rat,
		burst:        burst,
		ttl:          ttl,
		maxKeys:      maxKeys,
		enableGlobal: options.EnableGlobal,
	}
	for i := range r.shards {
		r.shards[i].items = make(map[string]*list.Element)
		r.shards[i].capacity = maxKeys / numShards
		if i < maxKeys%numShards {
			r.shards[i].capacity++
		}
	}
	if options.EnableGlobal {
		r.globalLimiter = rate.NewLimiter(options.GlobalRate, options.GlobalBurst)
	}

	return r
}

func (r *RateLimiter) shardFor(key string) *limiterShard {
	// FNV-1a
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &r.shards[h%uint32(len(r.shards))]
}

// limiterForKey returns the limiter for key, creating it if needed.
func (r *RateLimiter) limiterForKey(key string) *rate.Limiter {
	now := time.Now().UnixNano()
	s := r.shardFor(key)

	s.mu.Lock()
	defer s.mu.Unlock()

	if el, ok := s.items[key]; ok {
		entry := el.Value.(*limiterEntry)
		entry.lastUsed = now
		s.lru.MoveToFront(el)
		return entry.limiter
	}

	// Drop expired keys from the cold end. Each key is removed at most once,
	// so this is amortized O(1) per insert.
	if r.ttl > 0 {
		ttl := r.ttl.Nanoseconds()
		for el := s.lru.Back(); el != nil && now-el.Value.(*limiterEntry).lastUsed > ttl; el = s.lru.Back() {
			s.remove(el)
		}
	}
	if len(s.items) >= s.capacity {
		s.remove(s.lru.Back())
	}

	entry := &limiterEntry{key: key, limiter: rate.NewLimiter(r.rate, r.burst), lastUsed: now}
	s.items[key] = s.lru.PushFront(entry)
	return entry.limiter
}

func (s *limiterShard) remove(el *list.Element) {
	delete(s.items, el.Value.(*limiterEntry).key)
	s.lru.Remove(el)
}

// keyCount returns the number of tracked keys.
func (r *RateLimiter) keyCount() int {
	n := 0
	for i := range r.shards {
		r.shards[i].mu.Lock()
		n += len(r.shards[i].items)
		r.shards[i].mu.Unlock()
	}
	return n
}

// has reports whether key is tracked.
func (r *RateLimiter) has(key string) bool {
	s := r.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[key]
	return ok
}
