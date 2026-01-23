package golitekit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMiddlewareQueue(t *testing.T) {
	t.Run("creates empty queue", func(t *testing.T) {
		mq := NewMiddlewareQueue()
		if len(mq) != 0 {
			t.Errorf("expected empty queue, got %d middlewares", len(mq))
		}
	})

	t.Run("creates queue with middlewares", func(t *testing.T) {
		m1 := func(next http.Handler) http.Handler { return next }
		m2 := func(next http.Handler) http.Handler { return next }

		mq := NewMiddlewareQueue(m1, m2)
		if len(mq) != 2 {
			t.Errorf("expected 2 middlewares, got %d", len(mq))
		}
	})
}

func TestMiddlewareQueue_Use(t *testing.T) {
	t.Run("adds middleware to queue", func(t *testing.T) {
		mq := NewMiddlewareQueue()
		m1 := func(next http.Handler) http.Handler { return next }

		mq.Use(m1)
		if len(mq) != 1 {
			t.Errorf("expected 1 middleware, got %d", len(mq))
		}
	})

	t.Run("adds multiple middlewares", func(t *testing.T) {
		mq := NewMiddlewareQueue()
		m1 := func(next http.Handler) http.Handler { return next }
		m2 := func(next http.Handler) http.Handler { return next }
		m3 := func(next http.Handler) http.Handler { return next }

		mq.Use(m1, m2, m3)
		if len(mq) != 3 {
			t.Errorf("expected 3 middlewares, got %d", len(mq))
		}
	})
}

func TestMiddlewareQueue_Clone(t *testing.T) {
	t.Run("creates independent copy", func(t *testing.T) {
		m1 := func(next http.Handler) http.Handler { return next }
		mq := NewMiddlewareQueue(m1)

		cloned := mq.Clone()

		// Add to original
		m2 := func(next http.Handler) http.Handler { return next }
		mq.Use(m2)

		if len(mq) != 2 {
			t.Errorf("original queue should have 2 middlewares, got %d", len(mq))
		}
		if len(cloned) != 1 {
			t.Errorf("cloned queue should have 1 middleware, got %d", len(cloned))
		}
	})
}

func TestMiddlewareQueue_Apply(t *testing.T) {
	t.Run("applies middlewares in correct order", func(t *testing.T) {
		var order []string

		m1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m1-before")
				next.ServeHTTP(w, r)
				order = append(order, "m1-after")
			})
		}
		m2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m2-before")
				next.ServeHTTP(w, r)
				order = append(order, "m2-after")
			})
		}
		m3 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "m3-before")
				next.ServeHTTP(w, r)
				order = append(order, "m3-after")
			})
		}

		mq := NewMiddlewareQueue(m1, m2, m3)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		})

		wrapped := mq.Apply(handler)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		// Middlewares should execute in order: m1 -> m2 -> m3 -> handler -> m3 -> m2 -> m1
		expected := []string{"m1-before", "m2-before", "m3-before", "handler", "m3-after", "m2-after", "m1-after"}
		if len(order) != len(expected) {
			t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
		}
		for i, v := range expected {
			if order[i] != v {
				t.Errorf("order[%d] = %s, want %s", i, order[i], v)
			}
		}
	})

	t.Run("empty queue returns handler unchanged", func(t *testing.T) {
		mq := NewMiddlewareQueue()

		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})

		wrapped := mq.Apply(handler)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("middleware can short-circuit", func(t *testing.T) {
		handlerCalled := false

		authMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "" {
					w.WriteHeader(http.StatusUnauthorized)
					return // Short-circuit, don't call next
				}
				next.ServeHTTP(w, r)
			})
		}

		mq := NewMiddlewareQueue(authMiddleware)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})

		wrapped := mq.Apply(handler)

		// Request without auth header
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if handlerCalled {
			t.Error("handler should not be called when auth fails")
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("middleware can modify request", func(t *testing.T) {
		var receivedHeader string

		addHeaderMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("X-Custom", "injected")
				next.ServeHTTP(w, r)
			})
		}

		mq := NewMiddlewareQueue(addHeaderMiddleware)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("X-Custom")
		})

		wrapped := mq.Apply(handler)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if receivedHeader != "injected" {
			t.Errorf("header = %s, want injected", receivedHeader)
		}
	})

	t.Run("middleware can modify response", func(t *testing.T) {
		addHeaderMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Response-Header", "added")
				next.ServeHTTP(w, r)
			})
		}

		mq := NewMiddlewareQueue(addHeaderMiddleware)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := mq.Apply(handler)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("X-Response-Header") != "added" {
			t.Error("expected response header to be added")
		}
	})
}
