package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestNewRateLimiter(t *testing.T) {
	t.Run("creates limiter with default options", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		if rl.rate != 10 {
			t.Errorf("rate = %v, want 10", rl.rate)
		}
		if rl.burst != 5 {
			t.Errorf("burst = %d, want 5", rl.burst)
		}
		if rl.enableGlobal {
			t.Error("global limiter should be disabled by default")
		}
		if rl.ttl != DefaultRateLimiterTTL {
			t.Errorf("ttl = %v, want default %v", rl.ttl, DefaultRateLimiterTTL)
		}
		if rl.maxKeys != DefaultRateLimiterMaxKeys {
			t.Errorf("maxKeys = %d, want default %d", rl.maxKeys, DefaultRateLimiterMaxKeys)
		}
	})

	t.Run("creates limiter with global rate limit", func(t *testing.T) {
		rl := NewRateLimiter(10, 5,
			WithGlobalRateLimiter(100, 50),
		)

		if !rl.enableGlobal {
			t.Error("global limiter should be enabled")
		}
		if rl.globalLimiter == nil {
			t.Error("global limiter should be created")
		}
	})

	t.Run("creates limiter with TTL", func(t *testing.T) {
		rl := NewRateLimiter(10, 5,
			WithTTL(time.Minute),
		)

		if rl.ttl != time.Minute {
			t.Errorf("ttl = %v, want 1m", rl.ttl)
		}
	})

	t.Run("can disable TTL explicitly", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithoutTTL())

		if rl.ttl != 0 {
			t.Errorf("ttl = %v, want disabled ttl", rl.ttl)
		}
	})

	t.Run("creates limiter with max keys", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(2))

		if rl.maxKeys != 2 {
			t.Errorf("maxKeys = %d, want 2", rl.maxKeys)
		}
	})
}

func TestRateLimiter_LimiterForKey(t *testing.T) {
	t.Run("returns same limiter for same key", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		if rl.limiterForKey("user-1") != rl.limiterForKey("user-1") {
			t.Error("expected same limiter instance for same key")
		}
	})

	t.Run("returns different limiters for different keys", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		if rl.limiterForKey("user-1") == rl.limiterForKey("user-2") {
			t.Error("expected different limiter instances for different keys")
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(50))

		var wg sync.WaitGroup
		for i := 0; i < 200; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				rl.limiterForKey(fmt.Sprintf("user-%d", i%100)).Allow()
			}(i)
		}
		wg.Wait()
		if n := rl.keyCount(); n > 50 {
			t.Fatalf("tracked keys = %d, want <= 50", n)
		}
	})

	t.Run("sharded limiter stays within max keys", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(5000), WithoutTTL())
		if len(rl.shards) < 2 {
			t.Fatalf("shards = %d, want a sharded limiter", len(rl.shards))
		}
		for i := 0; i < 20000; i++ {
			rl.limiterForKey(fmt.Sprintf("k%d", i))
		}
		if n := rl.keyCount(); n > 5000 {
			t.Fatalf("tracked keys = %d, want <= 5000", n)
		}
	})

	t.Run("evicts least recently used key when full", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(2), WithoutTTL())

		rl.limiterForKey("a")
		rl.limiterForKey("b")
		rl.limiterForKey("a") // a is now more recent than b
		rl.limiterForKey("c") // evicts b

		if !rl.has("a") || rl.has("b") || !rl.has("c") {
			t.Fatalf("a=%v b=%v c=%v, want b evicted", rl.has("a"), rl.has("b"), rl.has("c"))
		}
	})

	t.Run("expired keys are dropped as new keys arrive", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(10), WithTTL(25*time.Millisecond))

		rl.limiterForKey("old")
		time.Sleep(50 * time.Millisecond)
		rl.limiterForKey("new")

		if rl.has("old") || !rl.has("new") {
			t.Fatalf("old=%v new=%v, want old dropped after TTL", rl.has("old"), rl.has("new"))
		}
	})
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		limiter := NewRateLimiter(rate.Limit(10), 5).limiterForKey("user-1")

		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i)
			}
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		limiter := NewRateLimiter(rate.Limit(1), 2).limiterForKey("user-1")

		limiter.Allow()
		limiter.Allow()

		if limiter.Allow() {
			t.Error("request exceeding burst should be blocked")
		}
	})
}

func TestRateLimiterAsMiddleware(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 10)

		handlerCalled := false
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)

		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		ctx := withContext(req.Context())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("handler should be called when within rate limit")
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 1)

		handlerCalled := 0
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled++
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			ctx := withContext(req.Context())
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)
		}

		if handlerCalled != 1 {
			t.Errorf("handler called %d times, want 1", handlerCalled)
		}
	})

	t.Run("returns 429 when rate limited", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 1)

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)

		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.RemoteAddr = "192.168.1.1:12345"
		wrapped.ServeHTTP(httptest.NewRecorder(), req1.WithContext(withContext(req1.Context())))

		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.RemoteAddr = "192.168.1.1:12345"
		rec2 := httptest.NewRecorder()
		wrapped.ServeHTTP(rec2, req2.WithContext(withContext(req2.Context())))

		if rec2.Code != http.StatusTooManyRequests {
			t.Errorf("status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
		}
	})

	t.Run("new clients are served when key capacity is full", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 10, WithMaxKeys(1), WithoutTTL())

		handlerCalled := 0
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled++
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)
		for _, addr := range []string{"192.168.1.1:1", "192.168.1.2:1", "192.168.1.3:1"} {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = addr
			if err := wrapped(req.Context(), httptest.NewRecorder(), req); err != nil {
				t.Fatalf("%s: error %v, want request served", addr, err)
			}
		}
		if handlerCalled != 3 || rl.keyCount() != 1 {
			t.Fatalf("handlerCalled = %d keys = %d, want 3 and 1", handlerCalled, rl.keyCount())
		}
	})

	t.Run("global rate limiter works", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 100,
			WithGlobalRateLimiter(rate.Limit(1), 1),
		)

		handlerCalled := 0
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled++
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)

		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1." + string(rune('1'+i)) + ":12345"
			ctx := withContext(req.Context())
			req = req.WithContext(ctx)

			wrapped.ServeHTTP(httptest.NewRecorder(), req)
		}

		if handlerCalled != 1 {
			t.Errorf("handler called %d times, want 1 (global limit)", handlerCalled)
		}
	})
}

func TestByIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	key := ByIP(req)
	if key != "192.168.1.100" {
		t.Errorf("key = %s, want 192.168.1.100", key)
	}
}

func TestByPathUsesRoutePattern(t *testing.T) {
	rl := NewRateLimiter(rate.Limit(100), 100)
	var keys []string
	r := NewRouter(nil)
	r.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			keys = append(keys, ByPath(req))
			return next(ctx, w, req)
		}
	}, rl.RateLimiterAsMiddleware(ByPath))
	r.GET("/users/{id}", func(ctx *Context) error { return nil })

	for _, path := range []string{"/users/1", "/users/2", "/users/random-xyz"} {
		r.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	for _, k := range keys {
		if k != "GET /users/{id}" {
			t.Fatalf("keys = %v, want the route pattern for every request", keys)
		}
	}
	if rl.keyCount() != 1 {
		t.Fatalf("tracked keys = %d, want 1 for one route", rl.keyCount())
	}
}

func TestByClientIP(t *testing.T) {
	key := ByClientIP("127.0.0.1", "10.0.0.0/8")

	cases := []struct {
		name   string
		remote string
		xff    []string
		realIP string
		want   string
	}{
		{"untrusted peer ignores headers", "203.0.113.9:1", []string{"1.2.3.4"}, "5.6.7.8", "203.0.113.9"},
		{"trusted proxy uses XFF", "127.0.0.1:1", []string{"198.51.100.7"}, "", "198.51.100.7"},
		{"rightmost untrusted hop wins", "127.0.0.1:1", []string{"6.6.6.6, 198.51.100.7, 10.1.2.3"}, "", "198.51.100.7"},
		{"multiple XFF headers", "127.0.0.1:1", []string{"6.6.6.6", "198.51.100.7"}, "", "198.51.100.7"},
		{"X-Real-IP without XFF", "10.0.0.5:1", nil, "198.51.100.8", "198.51.100.8"},
		{"malformed XFF falls back", "127.0.0.1:1", []string{"not-an-ip"}, "", "127.0.0.1"},
		{"no headers uses peer", "127.0.0.1:1", nil, "", "127.0.0.1"},
		{"all hops trusted", "127.0.0.1:1", []string{"10.0.0.1, 10.0.0.2"}, "", "10.0.0.1"},
		{"XFF hop with port", "127.0.0.1:1", []string{"198.51.100.7:5678"}, "", "198.51.100.7"},
		{"XFF IPv6 hop with port", "127.0.0.1:1", []string{"[2001:db8::1]:443"}, "", "2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if tc.realIP != "" {
				req.Header.Set("X-Real-IP", tc.realIP)
			}
			if got := key(req); got != tc.want {
				t.Fatalf("key = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestByClientIPPanicsOnInvalidProxy(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid trusted proxy")
		}
	}()
	ByClientIP("not-a-cidr/99")
}

// Trusted proxies given as IPv4-mapped prefixes or zoned link-local addresses
// still match.
func TestByClientIPNormalizesTrustedProxies(t *testing.T) {
	key := ByClientIP("::ffff:10.0.0.0/104", "fe80::1")
	for _, remote := range []string{"10.1.2.3:1", "[fe80::1%eth0]:1"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", "198.51.100.7")
		if got := key(req); got != "198.51.100.7" {
			t.Errorf("remote %s: key = %q, want the forwarded client", remote, got)
		}
	}
}
