package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hansir-hsj/GoLiteKit/env"
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
				return ErrTimeout(fmt.Sprintf("Request timeout: %v", context.Cause(timeoutCtx)))
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
	ctx := WithContext(req.Context())
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
	ctx := WithContext(req.Context())
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
	ctx := WithContext(req.Context())
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
	ctx := WithContext(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	err := wrapped(ctx, rec, req)

	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestTimeoutMiddleware_SSE(t *testing.T) {
	err := env.Init("env/app.toml")
	if err != nil {
		t.Skip("env not initialized, skipping test: " + err.Error())
	}

	t.Run("uses SSE timeout for event-stream requests", func(t *testing.T) {
		mw := TimeoutMiddleware()

		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			deadline, ok := ctx.Deadline()
			if ok {
				remaining := time.Until(deadline)
				t.Logf("SSE request has deadline in %v", remaining)
			}
			return nil
		})

		wrapped := mw(inner)

		req := httptest.NewRequest("GET", "/events", nil)
		req.Header.Set("Accept", "text/event-stream")
		ctx := WithContext(req.Context())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		wrapped(ctx, rec, req)
	})
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
	ctx := WithContext(req.Context())
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
