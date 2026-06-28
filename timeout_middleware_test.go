package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testTimeoutMiddleware(timeout time.Duration) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			if timeout < 1 {
				return next(ctx, w, r)
			}

			timeoutCtx, cancel := context.WithTimeoutCause(
				ctx,
				timeout,
				fmt.Errorf("request timeout after %v", timeout),
			)
			defer cancel()

			err := next(timeoutCtx, w, r.WithContext(timeoutCtx))

			if timeoutCtx.Err() == context.DeadlineExceeded && err == nil {
				return ErrTimeout(fmt.Sprintf("Request timeout: %v", context.Cause(timeoutCtx)), nil)
			}

			return err
		}
	}
}

func TestTimeoutMiddleware_CompletesBeforeTimeout(t *testing.T) {
	mw := testTimeoutMiddleware(5 * time.Second)

	handlerCalled := false
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		handlerCalled = true
		return nil
	})

	wrapped := mw(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeoutMiddleware_TimesOut(t *testing.T) {
	mw := testTimeoutMiddleware(100 * time.Millisecond)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		<-ctx.Done()
		return nil
	})

	wrapped := mw(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if err == nil {
		t.Error("expected timeout error")
	}
	appErr, ok := err.(*AppError)
	if !ok {
		t.Errorf("expected AppError, got %T", err)
	} else if appErr.Code != http.StatusRequestTimeout {
		t.Errorf("status = %d, want %d", appErr.Code, http.StatusRequestTimeout)
	}
}

func TestTimeoutMiddleware_ZeroTimeout(t *testing.T) {
	mw := testTimeoutMiddleware(0)

	handlerCalled := false
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		handlerCalled = true
		return nil
	})

	wrapped := mw(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeoutMiddleware_ContextCancellation(t *testing.T) {
	mw := testTimeoutMiddleware(200 * time.Millisecond)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		<-ctx.Done()
		return ctx.Err()
	})

	wrapped := mw(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestTimeoutMiddleware_NoOptionsDoesNotReadEnv(t *testing.T) {
	mw := TimeoutMiddleware()

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("expected no deadline without explicit timeout options")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	if err := mw(inner)(ctx, rec, req); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
}

func TestTimeoutMiddleware_ExplicitSSETimeout(t *testing.T) {
	mw := TimeoutMiddleware(TimeoutOptions{Duration: time.Minute, SSETimeout: 5 * time.Second})

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline for SSE request")
		}
		remaining := time.Until(deadline)
		if remaining > 6*time.Second {
			t.Fatalf("deadline remaining = %v, want SSE timeout", remaining)
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream")
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	if err := mw(inner)(ctx, rec, req); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
}

func TestTimeoutMiddlewareUsesExplicitDuration(t *testing.T) {
	mw := TimeoutMiddleware(TimeoutOptions{Duration: 5 * time.Second})
	called := false

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		called = true
		// Verify deadline is set
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected deadline to be set")
		}
		return nil
	})

	wrapped := mw(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}
