package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func TestLoggerAsMiddleware_NilLogger(t *testing.T) {
	called := false
	mw := LoggerAsMiddleware(nil, nil)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		called = true
		w.WriteHeader(http.StatusOK)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called")
	}
}

func TestLoggerAsMiddleware_ErrorPath(t *testing.T) {
	// When handler returns AppError, middleware must log a warning (no panic).
	mw := LoggerAsMiddleware(nil, nil)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return ErrBadRequest("test error", nil)
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	// Should not panic even when logger is nil and there is an error.
	mw(inner).ServeHTTP(rec, req)
}

func TestLoggerAsMiddleware_SuccessPath(t *testing.T) {
	responded := false
	mw := LoggerAsMiddleware(nil, nil)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		responded = true
		w.WriteHeader(http.StatusCreated)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if !responded {
		t.Error("expected handler to respond")
	}
}

func TestLoggerAsMiddleware_WithRealLogger(t *testing.T) {
	log, err := logger.NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	mw := LoggerAsMiddleware(log, nil)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
