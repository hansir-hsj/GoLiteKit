package golitekit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	req = req.WithContext(withContext(req.Context()))
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
	req = req.WithContext(logger.WithLoggerContext(withContext(req.Context())))
	rec := httptest.NewRecorder()

	// Should not panic even when logger is nil and there is an error.
	mw(inner).ServeHTTP(rec, req)
}

func TestLoggerAsMiddleware_RedactsInternalError(t *testing.T) {
	mw := LoggerAsMiddleware(nil, nil)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return ErrInternal("database failed", errors.New("dsn password=secret123 token=abc123"))
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req = req.WithContext(logger.WithLoggerContext(withContext(req.Context())))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	logCtx := logger.GetLoggerContext(req.Context())
	if logCtx == nil {
		t.Fatal("logger context missing")
	}
	for node := logCtx.Head; node != nil; node = node.Next {
		if node.Key != "err_internal" {
			continue
		}
		value := node.Value.(string)
		if strings.Contains(value, "secret123") || strings.Contains(value, "abc123") {
			t.Fatalf("err_internal leaked sensitive value: %s", value)
		}
		if !strings.Contains(value, "[REDACTED]") {
			t.Fatalf("err_internal was not redacted: %s", value)
		}
		return
	}
	t.Fatal("err_internal field missing")
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
	req = req.WithContext(withContext(req.Context()))
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
	req = req.WithContext(withContext(req.Context()))
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

func TestLoggerRedactSensitiveKeys_NestedArrays(t *testing.T) {
	input := `{"items":[{"username":"alice","access_token":"abc123"},{"profile":{"api_key":"key123","name":"bob"}}]}`
	result := redactSensitiveKeys(input)

	if strings.Contains(result, "abc123") || strings.Contains(result, "key123") {
		t.Fatalf("sensitive values inside arrays were not redacted: %s", result)
	}
	if !strings.Contains(result, "alice") || !strings.Contains(result, "bob") {
		t.Fatalf("non-sensitive values were redacted: %s", result)
	}
}

func TestLoggerSanitizeBody_InvalidJSONOmitted(t *testing.T) {
	input := []byte(`{"password":"secret123"`)
	result := sanitizeLoggedBody(input, DefaultLogBodyLimit, "application/json")

	if strings.Contains(result, "secret123") || strings.Contains(result, "password") {
		t.Fatalf("invalid json body leaked original content: %s", result)
	}
}

func TestLoggerSanitizeBody_RedactsBeforeTruncating(t *testing.T) {
	input := []byte(`{"password":"secret123","padding":"` + strings.Repeat("x", 5000) + `"}`)
	result := sanitizeLoggedBody(input, 64, "application/json")

	if strings.Contains(result, "secret123") {
		t.Fatalf("sensitive value leaked after truncation: %s", result)
	}
	if !strings.HasSuffix(result, "...(truncated)") {
		t.Fatalf("expected sanitized output to be truncated: %s", result)
	}
}

func TestLoggerSanitizeQuery_RedactsSensitiveValues(t *testing.T) {
	values := url.Values{}
	values.Set("token", "abc123")
	values.Set("password", "secret123")
	values.Set("page", "1")

	result := sanitizeQuery(values)

	if strings.Contains(result, "abc123") || strings.Contains(result, "secret123") {
		t.Fatalf("query leaked sensitive values: %s", result)
	}
	if !strings.Contains(result, "page=1") {
		t.Fatalf("non-sensitive query value missing: %s", result)
	}
}

func TestLoggerSanitizeURL_RedactsQueryValues(t *testing.T) {
	u, err := url.Parse("/login?token=abc123&page=1")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	result := sanitizeURL(u)

	if strings.Contains(result, "abc123") {
		t.Fatalf("url leaked sensitive query value: %s", result)
	}
	if !strings.HasPrefix(result, "/login?") || !strings.Contains(result, "page=1") {
		t.Fatalf("url lost non-sensitive data: %s", result)
	}
}

func TestLoggerSanitizeErrorMessage_RedactsSensitiveValues(t *testing.T) {
	input := `database failed: password=secret123 token=abc123 authorization=Bearer xyz`
	result := sanitizeErrorMessage(input, DefaultLogBodyLimit)

	for _, leaked := range []string{"secret123", "abc123", "Bearer xyz"} {
		if strings.Contains(result, leaked) {
			t.Fatalf("error message leaked %q: %s", leaked, result)
		}
	}
	if !strings.Contains(result, "database failed") {
		t.Fatalf("non-sensitive context missing: %s", result)
	}
}

func TestLoggerSanitizeResponseBody_RedactsJSON(t *testing.T) {
	input := []byte(`{"data":{"refresh_token":"rt123","name":"alice"}}`)
	result := sanitizeLoggedBody(input, DefaultLogBodyLimit, "application/json")

	if strings.Contains(result, "rt123") {
		t.Fatalf("response body leaked sensitive value: %s", result)
	}
	if !strings.Contains(result, "alice") {
		t.Fatalf("non-sensitive response value missing: %s", result)
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
