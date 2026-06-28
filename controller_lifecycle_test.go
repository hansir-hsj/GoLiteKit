package golitekit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type lifecycleRequest struct {
	Name string `json:"name" form:"name"`
}

type lifecycleRecorder struct {
	mu             sync.Mutex
	calls          []string
	validateName   string
	validateCalled bool
	serveCalled    bool
	finalizeCalled bool
}

func (r *lifecycleRecorder) append(call string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
}

func (r *lifecycleRecorder) markValidate(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validateCalled = true
	r.validateName = name
}

func (r *lifecycleRecorder) markServe() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serveCalled = true
}

func (r *lifecycleRecorder) markFinalize() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalizeCalled = true
}

func (r *lifecycleRecorder) snapshot() lifecycleRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	return lifecycleRecorder{
		calls:          append([]string(nil), r.calls...),
		validateName:   r.validateName,
		validateCalled: r.validateCalled,
		serveCalled:    r.serveCalled,
		finalizeCalled: r.finalizeCalled,
	}
}

type lifecycleOrderController struct {
	BaseControllerOf[lifecycleRequest]
	recorder *lifecycleRecorder
}

func (c *lifecycleOrderController) Init(ctx context.Context) error {
	c.recorder.append("Init")
	return c.BaseControllerOf.Init(ctx)
}

func (c *lifecycleOrderController) ParseRequest(ctx context.Context) error {
	c.recorder.append("ParseRequest")
	return c.BaseControllerOf.ParseRequest(ctx)
}

func (c *lifecycleOrderController) Validate(ctx context.Context) error {
	c.recorder.append("Validate")
	c.recorder.markValidate(c.Request.Name)
	return nil
}

func (c *lifecycleOrderController) Serve(ctx context.Context) error {
	c.recorder.append("Serve")
	c.recorder.markServe()
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func (c *lifecycleOrderController) Finalize(ctx context.Context) error {
	c.recorder.append("Finalize")
	c.recorder.markFinalize()
	return nil
}

func TestControllerLifecycle_OrderAndValidateSeesParsedRequest(t *testing.T) {
	recorder := &lifecycleRecorder{}
	r := newTestRouter()
	r.POST("/lifecycle", &lifecycleOrderController{recorder: recorder})

	req := httptest.NewRequest(http.MethodPost, "/lifecycle", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got := recorder.snapshot()
	wantCalls := []string{"Init", "ParseRequest", "Validate", "Serve", "Finalize"}
	if !reflect.DeepEqual(got.calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", got.calls, wantCalls)
	}
	if got.validateName != "alice" {
		t.Fatalf("Validate saw name %q, want alice", got.validateName)
	}
	if !got.serveCalled {
		t.Fatal("Serve was not called")
	}
	if !got.finalizeCalled {
		t.Fatal("Finalize was not called")
	}
}

type parseFailureController struct {
	BaseControllerOf[lifecycleRequest]
	recorder *lifecycleRecorder
}

func (c *parseFailureController) Validate(ctx context.Context) error {
	c.recorder.markValidate(c.Request.Name)
	return nil
}

func (c *parseFailureController) Serve(ctx context.Context) error {
	c.recorder.markServe()
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func (c *parseFailureController) Finalize(ctx context.Context) error {
	c.recorder.markFinalize()
	return nil
}

func TestControllerLifecycle_ParseFailureSkipsValidateServeFinalize(t *testing.T) {
	recorder := &lifecycleRecorder{}
	r := newTestRouter()
	r.POST("/parse-failure", &parseFailureController{recorder: recorder})

	req := httptest.NewRequest(http.MethodPost, "/parse-failure", strings.NewReader(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	got := recorder.snapshot()
	if got.validateCalled {
		t.Fatal("Validate was called after parse failure")
	}
	if got.serveCalled {
		t.Fatal("Serve was called after parse failure")
	}
	if got.finalizeCalled {
		t.Fatal("Finalize was called after parse failure")
	}
}

type validateFailureController struct {
	BaseControllerOf[lifecycleRequest]
	recorder *lifecycleRecorder
}

func (c *validateFailureController) Validate(ctx context.Context) error {
	c.recorder.markValidate(c.Request.Name)
	return ErrBadRequest("name is required", nil)
}

func (c *validateFailureController) Serve(ctx context.Context) error {
	c.recorder.markServe()
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func (c *validateFailureController) Finalize(ctx context.Context) error {
	c.recorder.markFinalize()
	return nil
}

func TestControllerLifecycle_ValidateFailureSkipsServeFinalize(t *testing.T) {
	recorder := &lifecycleRecorder{}
	r := newTestRouter()
	r.POST("/validate-failure", &validateFailureController{recorder: recorder})

	req := httptest.NewRequest(http.MethodPost, "/validate-failure", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	got := recorder.snapshot()
	if !got.validateCalled {
		t.Fatal("Validate was not called")
	}
	if got.validateName != "alice" {
		t.Fatalf("Validate saw name %q, want alice", got.validateName)
	}
	if got.serveCalled {
		t.Fatal("Serve was called after validate failure")
	}
	if got.finalizeCalled {
		t.Fatal("Finalize was called after validate failure")
	}
}

type noBodyMalformedJSONController struct {
	BaseController
}

func (c *noBodyMalformedJSONController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func TestControllerLifecycle_NoBodySkipsMalformedJSONParsing(t *testing.T) {
	r := newTestRouter()
	r.POST("/no-body", &noBodyMalformedJSONController{})

	req := httptest.NewRequest(http.MethodPost, "/no-body", strings.NewReader(`{"broken":`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body = %s", err, rec.Body.String())
	}
	if body["ok"] != "true" {
		t.Fatalf("response ok = %q, want true", body["ok"])
	}
}

type customParserRecorder struct {
	mu   sync.Mutex
	name string
}

func (r *customParserRecorder) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.name = name
}

func (r *customParserRecorder) snapshot() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.name
}

type customParserReadsRequestController struct {
	BaseController
	recorder *customParserRecorder
	name     string
}

func (c *customParserReadsRequestController) ParseRequest(ctx context.Context) error {
	rawBody, err := io.ReadAll(GetContext(ctx).Request().Body)
	if err != nil {
		return err
	}

	var payload lifecycleRequest
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return err
	}
	c.name = payload.Name
	c.recorder.record(c.name)
	return nil
}

func (c *customParserReadsRequestController) Validate(ctx context.Context) error {
	if c.name != "alice" {
		return ErrBadRequest("name is required", nil)
	}
	return nil
}

func (c *customParserReadsRequestController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
}

func TestControllerLifecycle_CustomRequestParserOwnsBodyParsing(t *testing.T) {
	recorder := &customParserRecorder{}
	r := newTestRouter()
	r.POST("/custom-parser", &customParserReadsRequestController{recorder: recorder})

	req := httptest.NewRequest(http.MethodPost, "/custom-parser", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	name := recorder.snapshot()
	if name != "alice" {
		t.Fatalf("custom parser name = %q, want alice", name)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v; body = %s", err, rec.Body.String())
	}
	if body["ok"] != "true" {
		t.Fatalf("response ok = %q, want true", body["ok"])
	}
}
