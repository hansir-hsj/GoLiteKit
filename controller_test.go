package golitekit

import (
	"bytes"
	"context"
	"mime/multipart"
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
	ctx := withContext(req.Context())
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))
	req = req.WithContext(ctx)
	return req, rec, ctx
}

// ============================================================================
// Init
// ============================================================================

func TestBaseController_Init_NoContext(t *testing.T) {
	// Init must fail when the request context has no golitekit Context.
	c := &BaseController{}
	err := c.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when context is not initialised")
	}
}

func TestBaseController_Init_WithFrameworkContext(t *testing.T) {
	req, rec, ctx := makeRequest(http.MethodGet, "/", nil, "")
	_ = ctx
	c := &BaseController{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	_ = rec
}

func TestBaseController_Init_DoesNotParseBody(t *testing.T) {
	body := []byte(`{"name":"alice","value":42}`)
	req, _, _ := makeRequest(http.MethodPost, "/", body, "application/json")

	c := &BaseControllerOf[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if len(c.gcx.RawBody()) != 0 {
		t.Fatalf("RawBody length = %d, want 0", len(c.gcx.RawBody()))
	}
	if c.Request != (jsonRequest{}) {
		t.Fatalf("Request = %+v, want zero value", c.Request)
	}
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

	c := &BaseControllerOf[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := c.ParseRequest(req.Context()); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Name != "alice" {
		t.Errorf("Name = %q, want %q", c.Request.Name, "alice")
	}
	if c.Request.Value != 42 {
		t.Errorf("Value = %d, want 42", c.Request.Value)
	}
}

func TestBaseController_ParseRequest_PopulatesRawBodyAndRequest(t *testing.T) {
	body := []byte(`{"name":"alice","value":42}`)
	req, _, _ := makeRequest(http.MethodPost, "/", body, "application/json")

	c := &BaseControllerOf[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if len(c.gcx.RawBody()) != 0 {
		t.Fatalf("RawBody length before ParseRequest = %d, want 0", len(c.gcx.RawBody()))
	}

	if err := c.ParseRequest(req.Context()); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	rawBody := c.gcx.RawBody()
	if !bytes.Equal(rawBody, body) {
		t.Fatalf("RawBody = %q, want %q", rawBody, body)
	}
	rawBody[0] = 'x'
	if !bytes.Equal(c.gcx.RawBody(), body) {
		t.Fatal("RawBody should return a copy")
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

	c := &BaseController{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := c.ParseRequest(req.Context()); err != nil {
		t.Errorf("unexpected error for NoBody controller: %v", err)
	}
}

func TestBaseController_ParseRequest_EmptyBody(t *testing.T) {
	// Empty body should not cause an error.
	req, _, _ := makeRequest(http.MethodPost, "/", nil, "application/json")

	c := &BaseControllerOf[jsonRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := c.ParseRequest(req.Context()); err != nil {
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

type multipartRequest struct {
	Title string `form:"title"`
}

func TestBaseController_Init_DoesNotParseForm(t *testing.T) {
	form := url.Values{}
	form.Set("username", "bob")
	form.Set("age", "30")

	req, _, _ := makeRequest(http.MethodPost, "/", []byte(form.Encode()), "application/x-www-form-urlencoded")

	c := &BaseControllerOf[formRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if req.Form != nil {
		t.Fatalf("req.Form = %v, want nil", req.Form)
	}
	if c.Request != (formRequest{}) {
		t.Fatalf("Request = %+v, want zero value", c.Request)
	}
}

func TestBaseController_ParseRequest_FormURLEncoded(t *testing.T) {
	form := url.Values{}
	form.Set("username", "bob")
	form.Set("age", "30")
	bodyStr := form.Encode()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bodyStr))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	ctx := withContext(req.Context())
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))
	req = req.WithContext(ctx)

	c := &BaseControllerOf[formRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// ParseRequest calls ParseForm and binds directly from request.Form.
	if err := c.ParseRequest(req.Context()); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Username != "bob" {
		t.Errorf("Username = %q, want %q", c.Request.Username, "bob")
	}
	if c.Request.Age != 30 {
		t.Errorf("Age = %d, want 30", c.Request.Age)
	}
}

func TestBaseController_ParseRequest_MultipartForm(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "upload"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fileWriter.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rec := httptest.NewRecorder()
	ctx := withContext(req.Context())
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))
	req = req.WithContext(ctx)

	c := &BaseControllerOf[multipartRequest]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if req.MultipartForm != nil {
		t.Fatalf("req.MultipartForm = %v, want nil", req.MultipartForm)
	}

	if err := c.ParseRequest(req.Context()); err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}

	if c.Request.Title != "upload" {
		t.Errorf("Title = %q, want %q", c.Request.Title, "upload")
	}
	file, header, err := c.FormFile("file")
	if err != nil {
		t.Fatalf("FormFile: %v", err)
	}
	defer file.Close()
	if header.Filename != "hello.txt" {
		t.Errorf("Filename = %q, want %q", header.Filename, "hello.txt")
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
	ctx := withContext(req.Context())
	gcx := GetContext(ctx)
	gcx.setContextOptions(withRequest(req.WithContext(ctx)), withResponseWriter(rec))
	req = req.WithContext(ctx)

	c := &BaseControllerOf[Req]{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// ParseRequest handles form types by parsing the form before binding fields.
	if err := c.ParseRequest(req.Context()); err != nil {
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

	c := &BaseController{}
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

	c := &BaseController{}
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

	c := &BaseController{}
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

	c := &BaseController{}
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

	c := &BaseController{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := c.BadRequest("invalid", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	appErr, ok := err.(*AppError)
	if !ok {
		t.Fatal("expected *AppError")
	}
	if appErr.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want %d", appErr.Code, http.StatusBadRequest)
	}
}

func TestBaseController_InternalError(t *testing.T) {
	req, _, _ := makeRequest(http.MethodGet, "/", nil, "")

	c := &BaseController{}
	if err := c.Init(req.Context()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	err := c.InternalError("oops", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	appErr, ok := err.(*AppError)
	if !ok {
		t.Fatal("expected *AppError")
	}
	if appErr.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want %d", appErr.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// Response helpers
// ============================================================================

type serveController struct {
	BaseController
}

func (c *serveController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"msg": "ok"})
}

func TestBaseController_JSON(t *testing.T) {
	req, rec, _ := makeRequest(http.MethodGet, "/", nil, "")

	r := newTestRouter()
	r.GET("/sj", &serveController{})

	r.Handler().ServeHTTP(rec, req)
	// Just confirm no panic and the recorder was used.
	_ = rec
}

func TestBaseController_ServiceReadsStartupRegisteredService(t *testing.T) {
	svc := &Services{}
	WithService("fake", &controllerFakeService{Name: "primary"})(svc)

	r := newTestRouter()
	r.services = svc
	r.GET("/svc", &serviceController{})

	req := httptest.NewRequest(http.MethodGet, "/svc", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

type serviceController struct {
	BaseController
}

type controllerFakeService struct{ Name string }

func (c *serviceController) Serve(ctx context.Context) error {
	got, ok := c.Service("fake").(*controllerFakeService)
	if !ok {
		return ErrInternal("service type mismatch", nil)
	}
	if got.Name != "primary" {
		return ErrInternal("service name mismatch", nil)
	}
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}
