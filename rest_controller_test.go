package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// Helpers shared by RestController tests
// ============================================================================

// buildRestCtx creates a minimal context + recorder ready for RestController methods.
func buildRestCtx(t *testing.T) (context.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))
	return ctx, rec
}

// ============================================================================
// ServeData
// ============================================================================

func TestRestController_ServeData(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	c.ServeData(ctx, map[string]int{"count": 7})

	// ServeData sets jsonResponse; render it through ContextAsMiddleware.
	mw := ContextAsMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != OK {
		t.Errorf("status = %d, want %d", resp.Status, OK)
	}
	if resp.Msg != "OK" {
		t.Errorf("msg = %q, want OK", resp.Msg)
	}
}

func TestRestController_ServeOK(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	c.ServeOK(ctx)

	mw := ContextAsMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != OK {
		t.Errorf("status = %d, want %d", resp.Status, OK)
	}
	if resp.Data != nil {
		t.Errorf("expected nil data for ServeOK, got %v", resp.Data)
	}
}

// ============================================================================
// ServeMsgData
// ============================================================================

func TestRestController_ServeMsgData(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	c.ServeMsgData(ctx, "custom message", "payload")

	mw := ContextAsMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Msg != "custom message" {
		t.Errorf("msg = %q, want %q", resp.Msg, "custom message")
	}
}

// ============================================================================
// ServeError / ServeErrorMsg
// ============================================================================

func TestRestController_ServeError(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	c.ServeError(ctx, -10, "something went wrong")

	mw := ContextAsMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != -10 {
		t.Errorf("status = %d, want -10", resp.Status)
	}
	if resp.Msg != "something went wrong" {
		t.Errorf("msg = %q, want %q", resp.Msg, "something went wrong")
	}
}

func TestRestController_ServeErrorMsg(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	c.ServeErrorMsg(ctx, "bad request from client")

	mw := ContextAsMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != -1 {
		t.Errorf("status = %d, want -1", resp.Status)
	}
}

// ============================================================================
// LogID is included when a Tracker is present
// ============================================================================

func TestRestController_ServeData_IncludesLogID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithContext(req.Context())
	ctx = WithTracker(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))

	c := &RestController{}
	c.gcx = gcx

	c.ServeData(ctx, nil)

	mw := ContextAsMiddleware()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req.WithContext(ctx))

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.LogID == "" {
		t.Error("expected LogID to be populated when Tracker is present")
	}
}
