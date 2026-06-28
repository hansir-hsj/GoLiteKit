package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testController struct {
	BaseController
}

func (c *testController) Serve(ctx context.Context) error { return nil }

type valueController struct {
	BaseController
}

func (c valueController) Serve(ctx context.Context) error { return nil }

type allValueReceiverController struct{}

func (c allValueReceiverController) MaxMemorySize() int64 { return DefaultMaxMemorySize }
func (c allValueReceiverController) MaxBodySize() int64   { return DefaultMaxBodySize }
func (c allValueReceiverController) Serve(ctx context.Context) error {
	return nil
}

type okJsonController struct {
	BaseController
}

func (c *okJsonController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
}

type createdJsonController struct {
	BaseController
}

func (c *createdJsonController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"created": true})
}

type emptyArrayController struct {
	BaseController
}

func (c *emptyArrayController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, []any{})
}

// newTestRouter returns a Router with the minimal middleware stack for tests.
func newTestRouter() *Router {
	r := NewRouter(nil)
	r.Use(ErrorHandlerMiddleware())
	r.Use(ContextAsMiddleware())
	return r
}

func TestRouter_GET(t *testing.T) {
	r := newTestRouter()
	r.GET("/hello", &okJsonController{})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("body = %s, want JSON containing \"ok\":true", rec.Body.String())
	}
}

func TestRouter_AttachesFrameworkContext(t *testing.T) {
	r := newTestRouter()
	r.GET("/ctx", HandlerFunc(func(ctx *Context) error {
		if ctx == nil {
			t.Fatal("expected framework context")
		}
		if ctx.Request() == nil || ctx.ResponseWriter() == nil {
			t.Fatal("expected request and response writer on framework context")
		}
		return ctx.JSON(http.StatusAccepted, map[string]bool{"ok": true})
	}))

	req := httptest.NewRequest(http.MethodGet, "/ctx", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want JSON containing ok=true", rec.Body.String())
	}
}

func TestRouter_POST(t *testing.T) {
	r := newTestRouter()
	r.POST("/submit", &createdJsonController{})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"created":true`) {
		t.Errorf("body = %s, want JSON containing \"created\":true", rec.Body.String())
	}
}

func TestRouter_RejectsValueControllerWithClearPanic(t *testing.T) {
	r := newTestRouter()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "controller must be a pointer to struct") {
			t.Fatalf("panic = %q, want pointer controller guidance", msg)
		}
		if !strings.Contains(msg, "golitekit.valueController") {
			t.Fatalf("panic = %q, want concrete controller type", msg)
		}
	}()

	r.GET("/value", valueController{})
}

func TestRouter_RejectsValueReceiverControllerWithClearPanic(t *testing.T) {
	r := newTestRouter()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "controller must be a pointer to struct") {
			t.Fatalf("panic = %q, want pointer controller guidance", msg)
		}
		if !strings.Contains(msg, "golitekit.allValueReceiverController") {
			t.Fatalf("panic = %q, want concrete controller type", msg)
		}
	}()

	r.GET("/value-receiver", allValueReceiverController{})
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	r := NewRouter(nil)
	r.GET("/only-get", &testController{})

	req := httptest.NewRequest(http.MethodPost, "/only-get", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestRouter_MethodNotAllowedUsesCurrentMiddleware(t *testing.T) {
	executed := false
	r := NewRouter(nil)
	r.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			executed = true
			return next(ctx, w, req)
		}
	})
	r.GET("/only-get", &testController{})

	req := httptest.NewRequest(http.MethodPost, "/only-get", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if !executed {
		t.Fatal("expected 405 handler to run current middleware")
	}
}

func TestRouter_NotFound(t *testing.T) {
	r := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

type badInitController struct {
	BaseController
}

func (c *badInitController) Init(ctx context.Context) error {
	return c.BaseController.Init(ctx)
}

func (c *badInitController) Serve(ctx context.Context) error { return nil }

func TestRouter_InitFailureSets500(t *testing.T) {
	r := NewRouter(nil)
	r.Use(ErrorHandlerMiddleware())
	r.Use(ContextAsMiddleware())
	r.GET("/bad", &badInitController{})

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code == 0 {
		t.Error("expected a non-zero status code")
	}
}

type serveErrorController struct {
	BaseController
}

func (c *serveErrorController) Serve(ctx context.Context) error {
	return ErrInternal("serve failed", nil)
}

func TestRouter_ServeError_PropagatedViaMiddleware(t *testing.T) {
	r := newTestRouter()
	r.GET("/err", &serveErrorController{})

	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Status != http.StatusInternalServerError {
		t.Errorf("response status = %d, want %d", resp.Status, http.StatusInternalServerError)
	}
}

func TestRouter_Any_RegistersAllMethods(t *testing.T) {
	r := NewRouter(nil)
	r.Any("/multi", &testController{})

	for _, method := range []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
	} {
		req := httptest.NewRequest(method, "/multi", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		if rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("method %s: got 405, want registered handler", method)
		}
	}
}

func TestRouter_Group_RoutesWithPrefix(t *testing.T) {
	r := newTestRouter()
	g := r.Group("/api")
	g.GET("/users", &emptyArrayController{})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `[]`) {
		t.Errorf("body = %s, want JSON containing []", rec.Body.String())
	}
}

func TestRouter_Use_MiddlewareExecutes(t *testing.T) {
	executed := false
	r := NewRouter(nil)
	r.Use(Middleware(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			executed = true
			return next(ctx, w, req)
		}
	}))
	r.GET("/mw", &testController{})

	req := httptest.NewRequest(http.MethodGet, "/mw", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if !executed {
		t.Error("expected global middleware to execute")
	}
}

func TestRouter_UseAfterRouteRegistrationPanics(t *testing.T) {
	r := NewRouter(nil)
	r.GET("/already-registered", &testController{})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "middleware must be registered before routes") {
			t.Fatalf("panic = %q, want middleware ordering guidance", msg)
		}
	}()

	r.Use(func(next Handler) Handler {
		return next
	})
}

func TestRouter_StaticUsesCurrentMiddlewareAndFreezesUse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write static file: %v", err)
	}

	executed := false
	r := NewRouter(nil)
	r.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
			executed = true
			return next(ctx, w, req)
		}
	})
	r.Static("/static", dir)

	req := httptest.NewRequest(http.MethodGet, "/static/hello.txt", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !executed {
		t.Fatal("expected static route to run current middleware")
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "middleware must be registered before routes") {
			t.Fatalf("panic = %q, want middleware ordering guidance", msg)
		}
	}()

	r.Use(func(next Handler) Handler {
		return next
	})
}

func TestRouterGroup_UseAfterRouteRegistrationPanics(t *testing.T) {
	r := NewRouter(nil)
	g := r.Group("/api")
	g.GET("/users", &testController{})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "group middleware must be registered before group routes") {
			t.Fatalf("panic = %q, want group middleware ordering guidance", msg)
		}
	}()

	g.Use(func(next Handler) Handler {
		return next
	})
}

func TestRouterGroup_UseAfterNestedGroupPanics(t *testing.T) {
	r := NewRouter(nil)
	g := r.Group("/api")
	_ = g.Group("/v1")

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "group middleware must be registered before nested groups or routes") {
			t.Fatalf("panic = %q, want nested group middleware ordering guidance", msg)
		}
	}()

	g.Use(func(next Handler) Handler {
		return next
	})
}

func TestHandlerFuncRouteWritesJSON(t *testing.T) {
	r := newTestRouter()
	r.GET("/hello", func(ctx *Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("body = %q, want hello", rec.Body.String())
	}
}

func TestHandlerFuncRouteReturnsAppError(t *testing.T) {
	r := newTestRouter()
	r.GET("/bad", func(ctx *Context) error {
		return ErrBadRequest("invalid request", nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid request") {
		t.Fatalf("body = %q, want invalid request", rec.Body.String())
	}
}

func TestRouteTargetClassifiesHandlerFuncSeparatelyFromController(t *testing.T) {
	target := newRouteTarget(HandlerFunc(func(ctx *Context) error {
		return nil
	}))

	if target.handler == nil {
		t.Fatal("expected HandlerFunc route target")
	}
	if target.controller != nil {
		t.Fatalf("controller = %T, want nil for HandlerFunc route", target.controller)
	}
}

type prototypeController struct {
	BaseController
	Prefix string
	Count  int
}

func (c *prototypeController) Serve(ctx context.Context) error {
	c.Count++
	return c.Bytes(http.StatusOK, []byte(c.Prefix+"ok"))
}

func TestControllerPrototypeFieldsArePreserved(t *testing.T) {
	r := newTestRouter()
	r.GET("/proto", &prototypeController{Prefix: "configured-"})

	req := httptest.NewRequest(http.MethodGet, "/proto", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Body.String() != "configured-ok" {
		t.Fatalf("body = %q, want configured-ok", rec.Body.String())
	}
}

type perRequestStateController struct {
	BaseController
	Prefix string
	Count  int
}

func (c *perRequestStateController) Serve(ctx context.Context) error {
	c.Count++
	return c.Bytes(http.StatusOK, []byte(c.Prefix+fmt.Sprint(c.Count)))
}

func TestControllerRequestStateDoesNotLeakAcrossRequests(t *testing.T) {
	r := newTestRouter()
	r.GET("/state", &perRequestStateController{Prefix: "configured-"})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/state", nil)
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)

		if rec.Body.String() != "configured-1" {
			t.Fatalf("request %d body = %q, want configured-1", i+1, rec.Body.String())
		}
	}
}

func TestControllerRequestTracksMiddlewareRequestContext(t *testing.T) {
	r := newTestRouter()
	r.Use(TimeoutMiddleware(TimeoutOptions{Duration: time.Second}))

	var sawDeadline bool
	r.GET("/deadline", func(ctx *Context) error {
		_, sawDeadline = ctx.Request().Context().Deadline()
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "yes"})
	})

	req := httptest.NewRequest(http.MethodGet, "/deadline", nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !sawDeadline {
		t.Fatal("expected Context.Request to include middleware-updated context")
	}
}
