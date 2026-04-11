package golitekit

import (
	"context"
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
		m1 := Middleware(func(next Handler) Handler { return next })
		m2 := Middleware(func(next Handler) Handler { return next })

		mq := NewMiddlewareQueue(m1, m2)
		if len(mq) != 2 {
			t.Errorf("expected 2 middlewares, got %d", len(mq))
		}
	})
}

func TestMiddlewareQueue_Use(t *testing.T) {
	t.Run("adds middleware to queue", func(t *testing.T) {
		mq := NewMiddlewareQueue()
		m1 := Middleware(func(next Handler) Handler { return next })

		mq.Use(m1)
		if len(mq) != 1 {
			t.Errorf("expected 1 middleware, got %d", len(mq))
		}
	})

	t.Run("adds multiple middlewares", func(t *testing.T) {
		mq := NewMiddlewareQueue()
		m1 := Middleware(func(next Handler) Handler { return next })
		m2 := Middleware(func(next Handler) Handler { return next })
		m3 := Middleware(func(next Handler) Handler { return next })

		mq.Use(m1, m2, m3)
		if len(mq) != 3 {
			t.Errorf("expected 3 middlewares, got %d", len(mq))
		}
	})
}

func TestMiddlewareQueue_Clone(t *testing.T) {
	t.Run("creates independent copy", func(t *testing.T) {
		m1 := Middleware(func(next Handler) Handler { return next })
		mq := NewMiddlewareQueue(m1)

		cloned := mq.Clone()

		m2 := Middleware(func(next Handler) Handler { return next })
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

		m1 := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				order = append(order, "m1-before")
				err := next(ctx, w, r)
				order = append(order, "m1-after")
				return err
			}
		})
		m2 := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				order = append(order, "m2-before")
				err := next(ctx, w, r)
				order = append(order, "m2-after")
				return err
			}
		})
		m3 := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				order = append(order, "m3-before")
				err := next(ctx, w, r)
				order = append(order, "m3-after")
				return err
			}
		})

		mq := NewMiddlewareQueue(m1, m2, m3)

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			order = append(order, "handler")
			return nil
		})

		wrapped := mq.Apply(inner)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

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
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			called = true
			return nil
		})

		wrapped := mq.Apply(inner)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("middleware can short-circuit by returning error", func(t *testing.T) {
		handlerCalled := false

		authMiddleware := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				if r.Header.Get("Authorization") == "" {
					return ErrUnauthorized("Unauthorized")
				}
				return next(ctx, w, r)
			}
		})

		mq := NewMiddlewareQueue(authMiddleware)

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			handlerCalled = true
			return nil
		})

		wrapped := mq.Apply(inner)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if handlerCalled {
			t.Error("handler should not be called when auth middleware short-circuits")
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("middleware can modify request", func(t *testing.T) {
		var receivedHeader string

		addHeaderMiddleware := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				r.Header.Set("X-Custom", "injected")
				return next(ctx, w, r)
			}
		})

		mq := NewMiddlewareQueue(addHeaderMiddleware)

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			receivedHeader = r.Header.Get("X-Custom")
			return nil
		})

		wrapped := mq.Apply(inner)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if receivedHeader != "injected" {
			t.Errorf("header = %s, want injected", receivedHeader)
		}
	})

	t.Run("middleware can modify response", func(t *testing.T) {
		addHeaderMiddleware := Middleware(func(next Handler) Handler {
			return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				w.Header().Set("X-Response-Header", "added")
				return next(ctx, w, r)
			}
		})

		mq := NewMiddlewareQueue(addHeaderMiddleware)

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		wrapped := mq.Apply(inner)

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Header().Get("X-Response-Header") != "added" {
			t.Error("expected response header to be added")
		}
	})
}

func TestStdMiddleware(t *testing.T) {
	t.Run("adapts net/http middleware", func(t *testing.T) {
		injected := false
		stdMW := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				injected = true
				next.ServeHTTP(w, r)
			})
		}

		mw := StdMiddleware(stdMW)
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		mw(inner).ServeHTTP(rec, req)

		if !injected {
			t.Error("expected std middleware to execute")
		}
	})

	t.Run("propagates error from inner handler", func(t *testing.T) {
		stdMW := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}

		mw := StdMiddleware(stdMW)
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return ErrBadRequest("bad", nil)
		})

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		err := mw(inner)(req.Context(), rec, req)

		if err == nil {
			t.Error("expected error to be propagated")
		}
	})
}
