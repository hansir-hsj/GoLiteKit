package golitekit

import (
	"context"
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
	t.Run("creates new limiter for new key", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected limiter to be available")
		}
		if limiter == nil {
			t.Fatal("expected limiter to be created")
		}
	})

	t.Run("returns same limiter for same key", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter1, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected first limiter to be available")
		}
		limiter2, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected second limiter to be available")
		}

		if limiter1 != limiter2 {
			t.Error("expected same limiter instance for same key")
		}
	})

	t.Run("returns different limiters for different keys", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter1, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected first limiter to be available")
		}
		limiter2, ok := rl.limiterForKey("user-2")
		if !ok {
			t.Fatal("expected second limiter to be available")
		}

		if limiter1 == limiter2 {
			t.Error("expected different limiter instances for different keys")
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				key := "user-" + string(rune('0'+i%10))
				_, _ = rl.limiterForKey(key)
			}(i)
		}
		wg.Wait()
	})

	t.Run("rejects new keys when max keys exceeded", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(1), WithoutTTL())

		if _, ok := rl.limiterForKey("user-1"); !ok {
			t.Fatal("expected first limiter to be available")
		}
		if _, ok := rl.limiterForKey("user-2"); ok {
			t.Fatal("expected second limiter to be rejected")
		}
	})

	t.Run("returns existing key even when max keys reached", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(1), WithoutTTL())

		first, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected first limiter to be available")
		}
		second, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected existing limiter to remain available")
		}
		if first != second {
			t.Fatal("expected same limiter for existing key")
		}
	})

	t.Run("expired keys are evicted before max key rejection", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithMaxKeys(1), WithTTL(25*time.Millisecond))

		if _, ok := rl.limiterForKey("user-1"); !ok {
			t.Fatal("expected first limiter to be available")
		}
		time.Sleep(50 * time.Millisecond)
		if _, ok := rl.limiterForKey("user-2"); !ok {
			t.Fatal("expected new limiter after expired key cleanup")
		}

		rl.mu.RLock()
		_, oldExists := rl.limiters["user-1"]
		_, newExists := rl.limiters["user-2"]
		rl.mu.RUnlock()
		if oldExists || !newExists {
			t.Fatalf("oldExists=%v newExists=%v, want old evicted and new created", oldExists, newExists)
		}
	})
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(10), 5)
		limiter, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected limiter to be available")
		}

		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i)
			}
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 2)
		limiter, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected limiter to be available")
		}

		limiter.Allow()
		limiter.Allow()

		if limiter.Allow() {
			t.Error("request exceeding burst should be blocked")
		}
	})
}

func TestRateLimiter_TTL(t *testing.T) {
	t.Run("removes limiter after TTL via lazy eviction", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithTTL(50*time.Millisecond))

		_, ok := rl.limiterForKey("user-1")
		if !ok {
			t.Fatal("expected limiter to be available")
		}

		rl.mu.RLock()
		_, exists := rl.limiters["user-1"]
		rl.mu.RUnlock()
		if !exists {
			t.Error("limiter should exist immediately after creation")
		}

		// Wait for TTL to expire
		time.Sleep(100 * time.Millisecond)

		// Trigger cleanExpired by calling cleanExpired directly
		rl.cleanExpired()

		rl.mu.RLock()
		_, exists = rl.limiters["user-1"]
		rl.mu.RUnlock()
		if exists {
			t.Error("limiter should be removed after TTL expiry and cleanup")
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

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(inner)

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

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(inner)

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

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(inner)

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

	t.Run("returns 429 when key capacity exceeded", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 10, WithMaxKeys(1), WithoutTTL())

		handlerCalled := 0
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled++
			return nil
		})

		wrapped := rl.RateLimiterAsMiddleware(ByIP)(inner)

		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.RemoteAddr = "192.168.1.1:12345"
		if err := wrapped(req1.Context(), httptest.NewRecorder(), req1); err != nil {
			t.Fatalf("first request error: %v", err)
		}

		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.RemoteAddr = "192.168.1.2:12345"
		err := wrapped(req2.Context(), httptest.NewRecorder(), req2)
		if appErr, ok := err.(*AppError); !ok || appErr.Code != http.StatusTooManyRequests {
			t.Fatalf("error = %#v, want 429 AppError", err)
		}
		if handlerCalled != 1 {
			t.Fatalf("handlerCalled = %d, want 1", handlerCalled)
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

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(inner)

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

func TestByPath(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/users", nil)

	key := ByPath(req)
	if key != "/api/users" {
		t.Errorf("key = %s, want /api/users", key)
	}
}
