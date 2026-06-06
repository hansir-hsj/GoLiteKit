package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func TestLogIDMiddlewareAddsLogIDToLoggerContext(t *testing.T) {
	mw := LogIDMiddleware()
	called := false

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		called = true
		loggerCtx := logger.GetLoggerContext(ctx)
		if loggerCtx == nil {
			t.Fatal("expected logger context")
		}
		for node := loggerCtx.Head; node != nil; node = node.Next {
			if node.Key == "logid" {
				if node.Value == "" {
					t.Fatal("expected non-empty logid field")
				}
				return nil
			}
		}
		t.Fatal("expected logid field in logger context")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/logid", nil)
	req = req.WithContext(WithContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
}
