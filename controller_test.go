package golitekit

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ============================================================================
// Helpers
// ============================================================================

// makeRequest builds an httptest.Request and attaches a golitekit context.
// The returned context is already wrapped with WithContext.
func makeRequest(method, path string, body []byte, contentType string) (*http.Request, *httptest.ResponseRecorder, context.Context) {
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	rec := httptest.NewRecorder()
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))
	req = req.WithContext(ctx)
	return req, rec, ctx
}

// ============================================================================
// Init
// ============================================================================

func TestBaseController_Init_NoContext(t *testing.T) {
	// Init must fail when the request context has no golitekit Context.
	c := &BaseController[NoBody]{}
	err := c.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when context is not initialised")
	}
}

func TestBaseController_Init_WithContext(t *testing.T) {
	req, rec, ctx := makeRequest(http.MethodGet, "/", nil, "")
	_ = ctx
	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	_ = rec
}

// ============================================================================
// parseBody / ParseRequest for JSON
// ============================================================================

type jsonRequest struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestBaseController_ParseRequest_JSON(t *testing.T) {
	body := []byte(`{"name":"alice","value":42}`)
	req, rec, _ := makeRequest(http.MethodPost, "/", body, "application/json")
	_ = rec

	c := &BaseController[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := c.ParseRequest(req.Context(), c.gcx.RawBody); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Name != "alice" {
		t.Errorf("Name = %q, want %q", c.Request.Name, "alice")
	}
	if c.Request.Value != 42 {
		t.Errorf("Value = %d, want 42", c.Request.Value)
	}
}

func TestBaseController_ParseRequest_NoBody(t *testing.T) {
	// NoBody controllers skip parsing regardless of body content.
	req, _, _ := makeRequest(http.MethodPost, "/", []byte(`{"x":1}`), "application/json")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := c.ParseRequest(req.Context(), []byte(`{"x":1}`)); err != nil {
		t.Errorf("unexpected error for NoBody controller: %v", err)
	}
}

func TestBaseController_ParseRequest_EmptyBody(t *testing.T) {
	// Empty body should not cause an error.
	req, _, _ := makeRequest(http.MethodPost, "/", nil, "application/json")

	c := &BaseController[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := c.ParseRequest(req.Context(), nil); err != nil {
		t.Errorf("unexpected error for empty body: %v", err)
	}
}

// ============================================================================
// Form binding
// ============================================================================

type formRequest struct {
	Username string `form:"username"`
	Age      int    `form:"age"`
}

func TestBaseController_ParseRequest_FormURLEncoded(t *testing.T) {
	form := url.Values{}
	form.Set("username", "bob")
	form.Set("age", "30")
	bodyStr := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))
	req = req.WithContext(ctx)

	c := &BaseController[formRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// parseBody calls ParseForm; ParseRequest now binds directly from request.Form.
	if err := c.ParseRequest(req.Context(), c.gcx.RawBody); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Username != "bob" {
		t.Errorf("Username = %q, want %q", c.Request.Username, "bob")
	}
	if c.Request.Age != 30 {
		t.Errorf("Age = %d, want 30", c.Request.Age)
	}
}

// ============================================================================
// setFieldValue
// ============================================================================

func TestSetFieldValue_PointerToString(t *testing.T) {
	type Req struct {
		Tag *string `form:"tag"`
	}

	form := url.Values{"tag": {"hello"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req.WithContext(ctx)), WithResponseWriter(rec))
	req = req.WithContext(ctx)

	c := &BaseController[Req]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// ParseRequest now handles form types correctly without relying on RawBody.
	if err := c.ParseRequest(req.Context(), c.gcx.RawBody); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Tag == nil {
		t.Fatal("expected *string field to be set, got nil")
	}
	if *c.Request.Tag != "hello" {
		t.Errorf("*Tag = %q, want %q", *c.Request.Tag, "hello")
	}
}

// ============================================================================
// Query helpers
// ============================================================================

func TestBaseController_QueryInt(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/?count=5", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if v := c.QueryInt("count", 0); v != 5 {
		t.Errorf("QueryInt count = %d, want 5", v)
	}
	if v := c.QueryInt("missing", 99); v != 99 {
		t.Errorf("QueryInt missing = %d, want 99 (default)", v)
	}
}

func TestBaseController_QueryString(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/?name=eve", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if v := c.QueryString("name", ""); v != "eve" {
		t.Errorf("QueryString = %q, want %q", v, "eve")
	}
	if v := c.QueryString("absent", "default"); v != "default" {
		t.Errorf("QueryString absent = %q, want %q", v, "default")
	}
}

func TestBaseController_QueryBool(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/?flag=1&flag2=true&flag3=false", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if v := c.QueryBool("flag", false); !v {
		t.Error("QueryBool flag=1: want true")
	}
	if v := c.QueryBool("flag2", false); !v {
		t.Error("QueryBool flag2=true: want true")
	}
	if v := c.QueryBool("flag3", true); v {
		t.Error("QueryBool flag3=false: want false")
	}
}

func TestBaseController_QueryFloat64(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/?pi=3.14", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	v := c.QueryFloat64("pi", 0)
	if v < 3.13 || v > 3.15 {
		t.Errorf("QueryFloat64 pi = %f, want ~3.14", v)
	}
}

// ============================================================================
// Error helpers
// ============================================================================

func TestBaseController_BadRequest(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := c.BadRequest("invalid", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	appErr := GetError(req.Context())
	if appErr == nil {
		t.Fatal("expected AppError in context")
	}
	if appErr.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want %d", appErr.Code, http.StatusBadRequest)
	}
}

func TestBaseController_HasError(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/", nil, "")

	c := &BaseController[NoBody]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if c.HasError() {
		t.Error("expected no error initially")
	}

	c.InternalError("oops", nil)
	if !c.HasError() {
		t.Error("expected HasError to return true after error is set")
	}
}

// ============================================================================
// ServeJSON / ServeRawData
// ============================================================================

type serveController struct {
	BaseController[NoBody]
}

func (c *serveController) Serve(ctx context.Context) error {
	return c.ServeJSON(map[string]string{"msg": "ok"})
}

func TestBaseController_ServeJSON(t *testing.T) {
	req, rec, _ := makeRequest(http.MethodGet, "/", nil, "")

	r := newTestRouter()
	r.GET("/sj", &serveController{})

	r.Handler().ServeHTTP(rec, req)
	// Just confirm no panic and the recorder was used.
	_ = rec
}
