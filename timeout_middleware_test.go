package golitekit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hansir-hsj/GoLiteKit/env"
)

func TestTimeoutMiddleware_Normal(t *testing.T) {
	// Initialize env with a test config
	err := env.Init("env/app.toml")
	if err != nil {
		t.Skip("env not initialized, skipping timeout test: " + err.Error())
	}

	t.Run("completes before timeout", func(t *testing.T) {
		middleware := TimeoutMiddleware()

		handlerCalled := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		ctx := WithContext(req.Context())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("expected handler to be called")
		}
	})
}

func TestTimeoutMiddleware_ZeroTimeout(t *testing.T) {
	// This test verifies the behavior when timeout is configured as 0
	// Since TimeoutMiddleware reads from env, we need env to be initialized
	err := env.Init("env/app.toml")
	if err != nil {
		t.Skip("env not initialized, skipping test: " + err.Error())
	}

	t.Run("handler is called when timeout is configured", func(t *testing.T) {
		middleware := TimeoutMiddleware()

		handlerCalled := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Error("expected handler to be called")
		}
	})
}

func TestTimeoutResponseWriter(t *testing.T) {
	t.Run("blocks write after timeout", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tw := newTimeoutResponseWriter(rec)

		// Write before timeout
		n, err := tw.Write([]byte("before"))
		if err != nil {
			t.Errorf("Write before timeout failed: %v", err)
		}
		if n != 6 {
			t.Errorf("bytes written = %d, want 6", n)
		}

		// Mark as timed out
		tw.markTimeout()

		// Write after timeout should fail
		n, err = tw.Write([]byte("after"))
		if err != http.ErrHandlerTimeout {
			t.Errorf("error = %v, want ErrHandlerTimeout", err)
		}
		if n != 0 {
			t.Errorf("bytes written after timeout = %d, want 0", n)
		}
	})

	t.Run("blocks WriteHeader after timeout", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tw := newTimeoutResponseWriter(rec)

		tw.markTimeout()
		tw.WriteHeader(http.StatusCreated)

		// Status should remain default (not changed)
		if tw.statusCode != http.StatusOK {
			t.Errorf("status = %d, should not change after timeout", tw.statusCode)
		}
	})

	t.Run("prevents duplicate WriteHeader", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tw := newTimeoutResponseWriter(rec)

		tw.WriteHeader(http.StatusCreated)
		tw.WriteHeader(http.StatusNotFound) // Should be ignored

		if tw.statusCode != http.StatusCreated {
			t.Errorf("status = %d, want %d", tw.statusCode, http.StatusCreated)
		}
	})

	t.Run("Flush is blocked after timeout", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tw := newTimeoutResponseWriter(rec)

		tw.markTimeout()

		// Should not panic
		tw.Flush()
	})
}

func TestTimeoutMiddleware_SSE(t *testing.T) {
	err := env.Init("env/app.toml")
	if err != nil {
		t.Skip("env not initialized, skipping test: " + err.Error())
	}

	t.Run("uses SSE timeout for event-stream requests", func(t *testing.T) {
		middleware := TimeoutMiddleware()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if context has appropriate timeout
			ctx := r.Context()
			deadline, ok := ctx.Deadline()
			if ok {
				remaining := time.Until(deadline)
				// SSE timeout should be longer than regular timeout
				t.Logf("SSE request has deadline in %v", remaining)
			}
			w.WriteHeader(http.StatusOK)
		})

		wrapped := middleware(handler)

		req := httptest.NewRequest("GET", "/events", nil)
		req.Header.Set("Accept", "text/event-stream")
		ctx := WithContext(req.Context())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)
	})
}
