package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func buildRestCtx(t *testing.T) (context.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))
	return ctx, rec
}

// renderContext runs ContextAsMiddleware with a no-op inner handler against ctx.
func renderContext(ctx context.Context, rec *httptest.ResponseRecorder) {
	mw := ContextAsMiddleware()
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	mw(inner).ServeHTTP(rec, req.WithContext(ctx))
}

func TestRestController_ServeData(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	if err := c.ServeData(ctx, map[string]int{"count": 7}); err != nil {
		t.Fatalf("ServeData: %v", err)
	}

	renderContext(ctx, rec)

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

	if err := c.ServeOK(ctx); err != nil {
		t.Fatalf("ServeOK: %v", err)
	}

	renderContext(ctx, rec)

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

func TestRestController_ServeMsgData(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	if err := c.ServeMsgData(ctx, "custom message", "payload"); err != nil {
		t.Fatalf("ServeMsgData: %v", err)
	}

	renderContext(ctx, rec)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Msg != "custom message" {
		t.Errorf("msg = %q, want %q", resp.Msg, "custom message")
	}
}

func TestRestController_ServeError(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	if err := c.ServeError(ctx, -10, "something went wrong"); err != nil {
		t.Fatalf("ServeError: %v", err)
	}

	renderContext(ctx, rec)

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

	if err := c.ServeErrorMsg(ctx, "bad request from client"); err != nil {
		t.Fatalf("ServeErrorMsg: %v", err)
	}

	renderContext(ctx, rec)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != -1 {
		t.Errorf("status = %d, want -1", resp.Status)
	}
}

func TestRestController_ServeData_IncludesLogID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithContext(req.Context())
	ctx = WithTracker(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))

	c := &RestController{}
	c.gcx = gcx

	if err := c.ServeData(ctx, nil); err != nil {
		t.Fatalf("ServeData: %v", err)
	}

	renderContext(ctx, rec)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.LogID == "" {
		t.Error("expected LogID to be populated when Tracker is present")
	}
}
