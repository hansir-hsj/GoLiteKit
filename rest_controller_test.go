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
	ctx := withContext(req.Context())
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))
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

func TestRestController_ServeErrorZeroStatusIsFailure(t *testing.T) {
	ctx, rec := buildRestCtx(t)
	c := &RestController{}
	c.gcx = GetContext(ctx)

	if err := c.ServeError(ctx, OK, "oops"); err != nil {
		t.Fatalf("ServeError: %v", err)
	}
	renderContext(ctx, rec)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != -1 {
		t.Errorf("status = %d, want -1 (0 is reserved for success)", resp.Status)
	}
}

func TestAppErrorBusinessStatusInErrorResponse(t *testing.T) {
	logs := &recordLogger{}
	app := NewApp(WithLogger(logs))
	app.GET("/biz", func(ctx *Context) error {
		return ErrBadRequest("invalid phone", nil).WithStatus(10012)
	})
	app.GET("/plain", func(ctx *Context) error {
		return ErrNotFound("missing", nil)
	})

	// A slice keeps the request order, which the log assertions below rely on.
	for _, tc := range []struct {
		path string
		want [2]int
	}{
		{"/biz", [2]int{http.StatusBadRequest, 10012}},
		{"/plain", [2]int{http.StatusNotFound, http.StatusNotFound}},
	} {
		path, want := tc.path, tc.want
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: unmarshal: %v", path, err)
		}
		if rec.Code != want[0] || resp.Status != want[1] {
			t.Errorf("%s: got HTTP %d status %d, want HTTP %d status %d", path, rec.Code, resp.Status, want[0], want[1])
		}
	}

	if got := logs.entries[0].fields["biz_status"]; got != 10012 {
		t.Errorf("biz_status log field = %v, want 10012", got)
	}
	if _, ok := logs.entries[1].fields["biz_status"]; ok {
		t.Error("biz_status should be absent when no business code is set")
	}
}

func TestRestController_ServeData_IncludesLogID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := withContext(req.Context())
	EnsureLogID(ctx)
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))

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
		t.Error("expected LogID to be populated when log ID is present")
	}
}
