package golitekit

import (
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
}

func TestRateLimiter_GetLimiter(t *testing.T) {
	t.Run("creates new limiter for new key", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter := rl.GetLimiter("user-1")
		if limiter == nil {
			t.Fatal("expected limiter to be created")
		}
	})

	t.Run("returns same limiter for same key", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter1 := rl.GetLimiter("user-1")
		limiter2 := rl.GetLimiter("user-1")

		if limiter1 != limiter2 {
			t.Error("expected same limiter instance for same key")
		}
	})

	t.Run("returns different limiters for different keys", func(t *testing.T) {
		rl := NewRateLimiter(10, 5)

		limiter1 := rl.GetLimiter("user-1")
		limiter2 := rl.GetLimiter("user-2")

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
				rl.GetLimiter(key)
			}(i)
		}
		wg.Wait()
	})
}

func TestRateLimiter_Allow(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(10), 5)
		limiter := rl.GetLimiter("user-1")

		// Burst of 5 should be allowed immediately
		for i := 0; i < 5; i++ {
			if !limiter.Allow() {
				t.Errorf("request %d should be allowed", i)
			}
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 2) // 1 req/sec, burst of 2
		limiter := rl.GetLimiter("user-1")

		// Use up burst
		limiter.Allow()
		limiter.Allow()

		// Next request should be denied
		if limiter.Allow() {
			t.Error("request exceeding burst should be blocked")
		}
	})
}

func TestRateLimiter_TTL(t *testing.T) {
	t.Run("removes limiter after TTL", func(t *testing.T) {
		rl := NewRateLimiter(10, 5, WithTTL(50*time.Millisecond))

		_ = rl.GetLimiter("user-1")

		// Limiter should exist
		rl.mu.RLock()
		_, exists := rl.limiters["user-1"]
		rl.mu.RUnlock()
		if !exists {
			t.Error("limiter should exist immediately after creation")
		}

		// Wait for TTL to expire
		time.Sleep(100 * time.Millisecond)

		// Limiter should be removed
		rl.mu.RLock()
		_, exists = rl.limiters["user-1"]
		rl.mu.RUnlock()
		if exists {
			t.Error("limiter should be removed after TTL")
		}
	})
}

func TestRateLimiterAsMiddleware(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 10)

		handlerCalled := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		ctx := WithContext(req.Context())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("handler should be called when within rate limit")
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 1) // Very restrictive

		handlerCalled := 0
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
		})

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(handler)

		// Make multiple requests quickly
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			ctx := WithContext(req.Context())
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)
		}

		// Only 1 request should have been allowed (burst of 1)
		if handlerCalled != 1 {
			t.Errorf("handler called %d times, want 1", handlerCalled)
		}
	})

	t.Run("sets error when rate limited", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(1), 1)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(handler)

		// Use up the limit
		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.RemoteAddr = "192.168.1.1:12345"
		ctx1 := WithContext(req1.Context())
		req1 = req1.WithContext(ctx1)
		wrapped.ServeHTTP(httptest.NewRecorder(), req1)

		// Second request should be rate limited
		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.RemoteAddr = "192.168.1.1:12345"
		ctx2 := WithContext(req2.Context())
		req2 = req2.WithContext(ctx2)
		rec2 := httptest.NewRecorder()

		wrapped.ServeHTTP(rec2, req2)

		err := GetError(ctx2)
		if err == nil {
			t.Error("expected error to be set when rate limited")
		}
		if err != nil && err.Code != http.StatusTooManyRequests {
			t.Errorf("error code = %d, want %d", err.Code, http.StatusTooManyRequests)
		}
	})

	t.Run("global rate limiter works", func(t *testing.T) {
		rl := NewRateLimiter(rate.Limit(100), 100,
			WithGlobalRateLimiter(rate.Limit(1), 1), // Global: 1 req/sec, burst 1
		)

		handlerCalled := 0
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
		})

		middleware := rl.RateLimiterAsMiddleware(ByIP)
		wrapped := middleware(handler)

		// Make requests from different IPs
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1." + string(rune('1'+i)) + ":12345"
			ctx := WithContext(req.Context())
			req = req.WithContext(ctx)

			wrapped.ServeHTTP(httptest.NewRecorder(), req)
		}

		// Only 1 should be allowed due to global limit
		if handlerCalled != 1 {
			t.Errorf("handler called %d times, want 1 (global limit)", handlerCalled)
		}
	})
}

func TestByIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	key := ByIP(req)
	if key != "192.168.1.100:54321" {
		t.Errorf("key = %s, want 192.168.1.100:54321", key)
	}
}

func TestByPath(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/users", nil)

	key := ByPath(req)
	if key != "/api/users" {
		t.Errorf("key = %s, want /api/users", key)
	}
}
