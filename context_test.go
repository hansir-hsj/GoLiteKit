package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestWithContext(t *testing.T) {
	t.Run("creates new context when none exists", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithContext(ctx)

		gcx := GetContext(newCtx)
		if gcx == nil {
			t.Fatal("expected Context to be created")
		}
		if gcx.data == nil {
			t.Error("expected data map to be initialized")
		}
	})

	t.Run("reuses existing context", func(t *testing.T) {
		ctx := context.Background()
		ctx1 := WithContext(ctx)
		gcx1 := GetContext(ctx1)

		ctx2 := WithContext(ctx1)
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
		ctx := WithContext(context.Background())
		gcx := GetContext(ctx)
		if gcx == nil {
			t.Error("expected Context to be returned")
		}
	})
}

func TestSetError_GetError(t *testing.T) {
	t.Run("sets and gets error", func(t *testing.T) {
		ctx := WithContext(context.Background())
		expectedErr := ErrBadRequest("test error", nil)

		SetError(ctx, expectedErr)
		gotErr := GetError(ctx)

		if gotErr == nil {
			t.Fatal("expected error to be returned")
		}
		if gotErr.Code != expectedErr.Code {
			t.Errorf("Code = %d, want %d", gotErr.Code, expectedErr.Code)
		}
		if gotErr.Message != expectedErr.Message {
			t.Errorf("Message = %s, want %s", gotErr.Message, expectedErr.Message)
		}
	})

	t.Run("returns nil when no error set", func(t *testing.T) {
		ctx := WithContext(context.Background())
		if GetError(ctx) != nil {
			t.Error("expected nil when no error is set")
		}
	})

	t.Run("returns nil for plain context", func(t *testing.T) {
		ctx := context.Background()
		if GetError(ctx) != nil {
			t.Error("expected nil for plain context")
		}
	})

	t.Run("handles invalid type in error key gracefully", func(t *testing.T) {
		ctx := WithContext(context.Background())
		// Manually set a non-AppError value
		SetContextData(ctx, AppErrorKey, "not an AppError")

		// Should return nil instead of panic
		err := GetError(ctx)
		if err != nil {
			t.Error("expected nil when stored value is not *AppError")
		}
	})
}

func TestHasError(t *testing.T) {
	t.Run("returns false when no error", func(t *testing.T) {
		ctx := WithContext(context.Background())
		if HasError(ctx) {
			t.Error("expected false when no error is set")
		}
	})

	t.Run("returns true when error exists", func(t *testing.T) {
		ctx := WithContext(context.Background())
		SetError(ctx, ErrInternal("test", nil))
		if !HasError(ctx) {
			t.Error("expected true when error is set")
		}
	})
}

func TestClearError(t *testing.T) {
	t.Run("clears existing error", func(t *testing.T) {
		ctx := WithContext(context.Background())
		SetError(ctx, ErrInternal("test", nil))

		if !HasError(ctx) {
			t.Fatal("expected error to be set")
		}

		ClearError(ctx)

		if HasError(ctx) {
			t.Error("expected error to be cleared")
		}
	})

	t.Run("does not panic when no error", func(t *testing.T) {
		ctx := WithContext(context.Background())
		// Should not panic
		ClearError(ctx)
	})
}

func TestSetContextData_GetContextData(t *testing.T) {
	t.Run("stores and retrieves data", func(t *testing.T) {
		ctx := WithContext(context.Background())

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
		ctx := WithContext(context.Background())

		_, ok := GetContextData(ctx, "non_existent")
		if ok {
			t.Error("expected false for non-existent key")
		}
	})

	t.Run("concurrent access is safe", func(t *testing.T) {
		ctx := WithContext(context.Background())
		var wg sync.WaitGroup

		// Concurrent writes
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				SetContextData(ctx, "key", i)
			}(i)
		}

		// Concurrent reads
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
		ctx := WithContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.SetContextOptions(WithRequest(req), WithResponseWriter(rec))
		gcx.ServeJSON(map[string]string{"status": "ok"})

		middleware := ContextAsMiddleware()
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handler does nothing, response is set via ServeJSON
		}))

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %s, want application/json", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != `{"status":"ok"}` {
			t.Errorf("body = %s, want {\"status\":\"ok\"}", rec.Body.String())
		}
	})

	t.Run("writes raw string response", func(t *testing.T) {
		ctx := WithContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.SetContextOptions(WithRequest(req), WithResponseWriter(rec))
		gcx.ServeRawData("hello world")

		middleware := ContextAsMiddleware()
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "text/plain; charset=UTF-8" {
			t.Errorf("Content-Type = %s, want text/plain; charset=UTF-8", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != "hello world" {
			t.Errorf("body = %s, want hello world", rec.Body.String())
		}
	})

	t.Run("writes HTML response", func(t *testing.T) {
		ctx := WithContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.SetContextOptions(WithRequest(req), WithResponseWriter(rec))
		gcx.ServeHTML("<h1>Hello</h1>")

		middleware := ContextAsMiddleware()
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		handler.ServeHTTP(rec, req)

		if rec.Header().Get("Content-Type") != "text/html; charset=UTF-8" {
			t.Errorf("Content-Type = %s, want text/html; charset=UTF-8", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("does not write when error exists", func(t *testing.T) {
		ctx := WithContext(context.Background())
		gcx := GetContext(ctx)

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		gcx.SetContextOptions(WithRequest(req), WithResponseWriter(rec))
		gcx.ServeJSON(map[string]string{"status": "ok"})
		SetError(ctx, ErrBadRequest("error", nil))

		middleware := ContextAsMiddleware()
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		handler.ServeHTTP(rec, req)

		// Should not write anything when error exists
		if rec.Body.Len() > 0 {
			t.Error("expected no body when error exists")
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
		// Check contains expected parts
		if !contains(body, "id: 123") {
			t.Error("expected id field")
		}
		if !contains(body, "event: message") {
			t.Error("expected event field")
		}
		if !contains(body, "retry: 3000") {
			t.Error("expected retry field")
		}
		if !contains(body, "data: test data") {
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
		if !contains(body, `{"count":42}`) {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
