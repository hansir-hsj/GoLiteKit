package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestWithFrameworkContext(t *testing.T) {
	t.Run("creates new context when none exists", func(t *testing.T) {
		ctx := context.Background()
		newCtx := withContext(ctx)

		gcx := GetContext(newCtx)
		if gcx == nil {
			t.Fatal("expected Context to be created")
		}
		SetContextData(newCtx, "k", 1)
		if _, ok := GetContextData(newCtx, "k"); !ok {
			t.Error("expected data to be retrievable after SetContextData")
		}
	})

	t.Run("reuses existing context", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := withContext(ctx)
		gcx1 := GetContext(ctx1)

		ctx2 := withContext(ctx1)
		gcx2 := GetContext(ctx2)

		if gcx1 != gcx2 {
			t.Error("expected same Context instance to be reused")
		}
	})
}

func TestGetContext(t *testing.T) {
	t.Run("returns nil for plain context", func(t *testing.T) {
		ctx := context.Background()
		gcx := GetContext(ctx)
		if gcx != nil {
			t.Error("expected nil for context without golitekit Context")
		}
	})

	t.Run("returns Context when present", func(t *testing.T) {
		ctx := withContext(context.Background())
		gcx := GetContext(ctx)
		if gcx == nil {
			t.Error("expected Context to be returned")
		}
	})
}

func TestRequestContextRemainsReadableAfterHandlerReturns(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/first", nil)
	glkCtx := newContext(req)
	ctx := req.WithContext(glkCtx).Context()
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req))
	SetContextData(ctx, "trace", "first")

	if got := GetContext(ctx); got != gcx {
		t.Fatal("request context should still resolve the original Context")
	}
	if gotReq := gcx.Request(); gotReq == nil || gotReq.URL.Path != "/first" {
		t.Fatalf("request after handler return = %v, want /first", gotReq)
	}
	if value, ok := GetContextData(ctx, "trace"); !ok || value != "first" {
		t.Fatalf("context data after handler return = %v, %v; want first, true", value, ok)
	}
}

func TestSetContextData_GetContextData(t *testing.T) {
	t.Run("stores and retrieves data", func(t *testing.T) {
		ctx := withContext(context.Background())

		SetContextData(ctx, "user_id", 12345)
		val, ok := GetContextData(ctx, "user_id")

		if !ok {
			t.Fatal("expected data to be found")
		}
		if val.(int) != 12345 {
			t.Errorf("value = %v, want 12345", val)
		}
	})

	t.Run("returns false for non-existent key", func(t *testing.T) {
		ctx := withContext(context.Background())

		_, ok := GetContextData(ctx, "non_existent")
		if ok {
			t.Error("expected false for non-existent key")
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		ctx := withContext(context.Background())
		var wg sync.WaitGroup

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SetContextData(ctx, "key", i)
			}(i)
		}

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				GetContextData(ctx, "key")
			}()
		}

		wg.Wait()
	})
}

func TestContextAsMiddleware(t *testing.T) {
	t.Run("writes JSON response", func(t *testing.T) {
		ctx := withContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.setContextOptions(withRequest(req), withResponseWriter(rec))
		if err := gcx.JSON(http.StatusOK, map[string]string{"status": "ok"}); err != nil {
			t.Fatalf("JSON failed: %v", err)
		}

		mw := ContextAsMiddleware()
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return nil
		})
		mw(inner).ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != `{"status":"ok"}` {
			t.Errorf("body = %s, want {\"status\":\"ok\"}", rec.Body.String())
		}
	})

	t.Run("writes raw string response", func(t *testing.T) {
		ctx := withContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.setContextOptions(withRequest(req), withResponseWriter(rec))
		if err := gcx.String(http.StatusOK, "hello world"); err != nil {
			t.Fatalf("String failed: %v", err)
		}

		mw := ContextAsMiddleware()
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return nil
		})
		mw(inner).ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "text/plain; charset=UTF-8" {
			t.Errorf("Content-Type = %s, want text/plain; charset=UTF-8", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != "hello world" {
			t.Errorf("body = %s, want hello world", rec.Body.String())
		}
	})

	t.Run("writes HTML response", func(t *testing.T) {
		ctx := withContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.setContextOptions(withRequest(req), withResponseWriter(rec))
		if err := gcx.HTML(http.StatusOK, "<h1>Hello</h1>"); err != nil {
			t.Fatalf("HTML failed: %v", err)
		}

		mw := ContextAsMiddleware()
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return nil
		})
		mw(inner).ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "text/html; charset=UTF-8" {
			t.Errorf("Content-Type = %s, want text/html; charset=UTF-8", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("propagates error without writing response", func(t *testing.T) {
		ctx := withContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.setContextOptions(withRequest(req), withResponseWriter(rec))
		if err := gcx.JSON(http.StatusOK, map[string]string{"status": "ok"}); err != nil {
			t.Fatalf("JSON failed: %v", err)
		}

		mw := ContextAsMiddleware()
		inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return ErrBadRequest("error", nil)
		})

		// Call the handler directly (not via ServeHTTP) to capture the returned error.
		err := mw(inner)(req.Context(), rec, req)

		if err == nil {
			t.Error("expected error to be propagated")
		}
		if rec.Body.Len() > 0 {
			t.Error("expected no body when handler returns error")
		}
	})
}

func TestSSEWriter(t *testing.T) {
	t.Run("sends basic event", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sse := NewSSEWriter(rec)

		err := sse.Send(SSEvent{Data: "hello"})
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		expected := "data: hello\n\n"
		if rec.Body.String() != expected {
			t.Errorf("body = %q, want %q", rec.Body.String(), expected)
		}
	})

	t.Run("sends event with all fields", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sse := NewSSEWriter(rec)

		err := sse.Send(SSEvent{
			ID:    "123",
			Event: "message",
			Data:  "test data",
			Retry: 3000,
		})
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		body := rec.Body.String()
		if body == "" {
			t.Error("expected non-empty body")
		}
		if !strings.Contains(body, "id: 123") {
			t.Error("expected id field")
		}
		if !strings.Contains(body, "event: message") {
			t.Error("expected event field")
		}
		if !strings.Contains(body, "retry: 3000") {
			t.Error("expected retry field")
		}
		if !strings.Contains(body, "data: test data") {
			t.Error("expected data field")
		}
	})

	t.Run("serializes JSON data", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sse := NewSSEWriter(rec)

		err := sse.Send(SSEvent{Data: map[string]int{"count": 42}})
		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		body := rec.Body.String()
		if !strings.Contains(body, `{"count":42}`) {
			t.Errorf("expected JSON data in body: %s", body)
		}
	})

	t.Run("sets correct headers", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewSSEWriter(rec)

		if rec.Header().Get("Content-Type") != "text/event-stream" {
			t.Error("expected Content-Type: text/event-stream")
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Error("expected Cache-Control: no-cache")
		}
	})
}

func TestContextServiceReadsStartupRegisteredService(t *testing.T) {
	type fakeService struct{ Name string }

	svc := &Services{}
	WithService("fake", &fakeService{Name: "primary"})(svc)
	r := newTestRouter()
	r.services = svc

	r.GET("/svc", func(ctx *Context) error {
		got, ok := ctx.Service("fake").(*fakeService)
		if !ok {
			return ErrInternal("service type mismatch", nil)
		}
		if got.Name != "primary" {
			return ErrInternal("service name mismatch", nil)
		}
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodGet, "/svc", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
