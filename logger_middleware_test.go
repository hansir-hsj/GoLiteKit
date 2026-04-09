package golitekit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

// ============================================================================
// LoggerAsMiddleware
// ============================================================================

func TestLoggerAsMiddleware_NilLogger(t *testing.T) {
	// Passing nil loggers must not panic; the middleware should still call next.
	called := false
	mw := LoggerAsMiddleware(nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called")
	}
}

func TestLoggerAsMiddleware_ErrorPath(t *testing.T) {
	// When an AppError is set, the middleware must log a warning (no panic).
	mw := LoggerAsMiddleware(nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetError(r.Context(), ErrBadRequest("test error", nil))
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	// Should not panic even when logger is nil and there is an error.
	mw(handler).ServeHTTP(rec, req)
}

func TestLoggerAsMiddleware_SuccessPath(t *testing.T) {
	// Successful requests must still call the underlying handler.
	responded := false
	mw := LoggerAsMiddleware(nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responded = true
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if !responded {
		t.Error("expected handler to respond")
	}
}

func TestLoggerAsMiddleware_WithRealLogger(t *testing.T) {
	// Smoke-test: a real console logger must not panic.
	log, err := logger.NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	mw := LoggerAsMiddleware(log, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
