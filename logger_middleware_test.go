package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestLoggerBodyTruncation(t *testing.T) {
	body := strings.Repeat("a", 5000)
	result := truncateBody([]byte(body), DefaultLogBodyLimit)

	if int64(len(result)) > DefaultLogBodyLimit+20 {
		t.Fatalf("truncated body too long: %d", len(result))
	}
	if !strings.HasSuffix(result, "...(truncated)") {
		t.Fatal("expected truncation suffix")
	}
}

func TestLoggerBodyTruncation_Short(t *testing.T) {
	body := "short body"
	result := truncateBody([]byte(body), DefaultLogBodyLimit)

	if result != body {
		t.Fatalf("result = %q, want %q", result, body)
	}
}

func TestLoggerRedactSensitiveKeys(t *testing.T) {
	input := `{"username":"alice","password":"secret123","token":"abc"}`
	result := redactSensitiveKeys(input)

	if strings.Contains(result, "secret123") {
		t.Fatalf("password not redacted: %s", result)
	}
	if strings.Contains(result, `"abc"`) {
		t.Fatalf("token not redacted: %s", result)
	}
	if !strings.Contains(result, "alice") {
		t.Fatalf("non-sensitive field was redacted: %s", result)
	}
}

func TestLoggerRedactSensitiveKeys_CaseInsensitive(t *testing.T) {
	input := `{"Password":"mypass","SECRET":"key","Authorization":"Bearer xyz"}`
	result := redactSensitiveKeys(input)

	if strings.Contains(result, "mypass") || strings.Contains(result, `"key"`) || strings.Contains(result, "Bearer") {
		t.Fatalf("sensitive keys not redacted (case-insensitive): %s", result)
	}
}

func TestLoggerIsLoggableContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"text/plain", true},
		{"", true},
		{"multipart/form-data", false},
		{"application/octet-stream", false},
		{"image/png", false},
	}
	for _, tt := range tests {
		if got := isLoggableContentType(tt.ct); got != tt.want {
			t.Errorf("isLoggableContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}
