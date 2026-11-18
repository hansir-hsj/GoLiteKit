package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	req = req.WithContext(withContext(req.Context()))
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next handler to be called")
	}
}

func TestIsValidLogID(t *testing.T) {
	for _, s := range []string{"abc", "req-1.2_3:4", strings.Repeat("a", 64)} {
		if !isValidLogID(s) {
			t.Errorf("isValidLogID(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", strings.Repeat("a", 65), "a b", "id\nlevel=ERROR", `x"y`, "中文"} {
		if isValidLogID(s) {
			t.Errorf("isValidLogID(%q) = true, want false", s)
		}
	}
}

func serveLogID(t *testing.T, app *App, path, incoming string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if incoming != "" {
		req.Header.Set(DefaultLogIDHeader, incoming)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDefaultStackPropagatesLogID(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.GET("/ok", func(ctx *Context) error { return ctx.String(http.StatusOK, EnsureLogID(ctx.Request().Context())) })
	app.GET("/err", func(ctx *Context) error { return ErrBadRequest("bad", nil) })
	app.GET("/panic", func(ctx *Context) error { panic("boom") })

	// By default an incoming ID is not trusted: a new one is generated and the
	// incoming one is logged for correlation.
	rec := serveLogID(t, app, "/ok", "gw-123")
	id := rec.Header().Get(DefaultLogIDHeader)
	if id == "" || id == "gw-123" || rec.Body.String() != id {
		t.Fatalf("want a generated id: header %q body %q", id, rec.Body.String())
	}
	entry := logs.only(t)
	if entry.fields["logid"] != id || entry.fields["upstream_request_id"] != "gw-123" {
		t.Fatalf("log fields = %+v, want generated logid and upstream_request_id gw-123", entry.fields)
	}

	logs.entries = nil
	rec = serveLogID(t, app, "/ok", "bad id\n")
	if got := rec.Header().Get(DefaultLogIDHeader); got == "" || strings.ContainsAny(got, " \n") {
		t.Fatalf("invalid incoming id should be replaced, got %q", got)
	}
	if _, ok := logs.only(t).fields["upstream_request_id"]; ok {
		t.Fatal("invalid incoming id must not be logged")
	}

	for _, path := range []string{"/err", "/panic", "/missing"} {
		rec := serveLogID(t, app, path, "")
		id := rec.Header().Get(DefaultLogIDHeader)
		var resp Response
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if id == "" || resp.LogID != id {
			t.Errorf("%s: header %q body logid %q, want same non-empty id", path, id, resp.LogID)
		}
	}
}

func TestLogIDOptionsTrustIncoming(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs), WithLogIDOptions(LogIDOptions{TrustIncoming: true}))
	app.GET("/ok", func(ctx *Context) error { return ctx.String(http.StatusOK, EnsureLogID(ctx.Request().Context())) })

	rec := serveLogID(t, app, "/ok", "gw-123")
	if rec.Header().Get(DefaultLogIDHeader) != "gw-123" || rec.Body.String() != "gw-123" {
		t.Fatalf("incoming id not reused: header %q body %q", rec.Header().Get(DefaultLogIDHeader), rec.Body.String())
	}
	entry := logs.only(t)
	if entry.fields["logid"] != "gw-123" {
		t.Fatalf("logid log field = %v, want gw-123", entry.fields["logid"])
	}
	if _, ok := entry.fields["upstream_request_id"]; ok {
		t.Fatal("upstream_request_id duplicates the reused logid")
	}
}

func TestLogIDOptionsOmitHeader(t *testing.T) {
	app := NewApp(WithLogIDOptions(LogIDOptions{OmitResponseHeader: true}))
	app.GET("/ok", func(ctx *Context) error { return ctx.String(http.StatusOK, EnsureLogID(ctx.Request().Context())) })

	rec := serveLogID(t, app, "/ok", "gw-123")
	if rec.Body.String() == "gw-123" || rec.Body.String() == "" {
		t.Fatalf("log id = %q, want a generated id", rec.Body.String())
	}
	if rec.Header().Get(DefaultLogIDHeader) != "" {
		t.Fatal("response header should be omitted")
	}
}

func TestLogIDCustomHeader(t *testing.T) {
	mw := LogIDMiddleware(LogIDOptions{Header: "X-Trace-Id", TrustIncoming: true})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Trace-Id", "t-1")
	req = req.WithContext(withContext(req.Context()))
	rec := httptest.NewRecorder()
	mw(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error { return nil }).ServeHTTP(rec, req)

	if rec.Header().Get("X-Trace-Id") != "t-1" || rec.Header().Get(DefaultLogIDHeader) != "" {
		t.Fatalf("headers = %v", rec.Header())
	}
}
