package golitekit

import (
	"context"
	"errors"
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

func TestTimeoutMiddleware_SSEExtendsWriteDeadline(t *testing.T) {
	mw := TimeoutMiddleware(TimeoutOptions{Duration: time.Second, SSETimeout: 5 * time.Minute})
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return NewSSEWriter(w).Send(SSEvent{Data: "hi"})
	})

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req.Header.Set("Accept", "text/event-stream, */*")
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	if err := mw(inner)(ctx, rec, req); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if remaining := time.Until(rec.writeDeadline); remaining < 4*time.Minute {
		t.Fatalf("write deadline remaining = %v, want about SSETimeout", remaining)
	}
}

// The Accept header alone must not lift the write deadline: a client could
// send it to any route.
func TestTimeoutMiddleware_SSEAcceptWithoutStreamKeepsWriteDeadline(t *testing.T) {
	mw := TimeoutMiddleware(TimeoutOptions{Duration: time.Second, SSETimeout: 5 * time.Minute})
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{}`))
		return err
	})

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Accept", "text/event-stream")
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}

	if err := mw(inner)(ctx, rec, req); err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !rec.writeDeadline.IsZero() {
		t.Fatalf("write deadline changed to %v for a non-SSE response", rec.writeDeadline)
	}
}

// A handler returning context.Cause(ctx) also timed out.
func TestTimeoutMiddleware_CauseErrorIsTimeout(t *testing.T) {
	mw := TimeoutMiddleware(TimeoutOptions{Duration: 10 * time.Millisecond})
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		<-ctx.Done()
		return context.Cause(ctx)
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	ctx := withContext(req.Context())
	req = req.WithContext(ctx)

	err := mw(inner)(ctx, httptest.NewRecorder(), req)
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503", err)
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
