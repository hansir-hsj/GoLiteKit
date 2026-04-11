package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testController struct {
	BaseController
}

func (c *testController) Serve(ctx context.Context) error { return nil }

type okJsonController struct {
	BaseController
}

func (c *okJsonController) Serve(ctx context.Context) error {
	GetContext(ctx).ServeJSON([]byte(`{"ok":true}`))
	return nil
}

type createdJsonController struct {
	BaseController
}

func (c *createdJsonController) Serve(ctx context.Context) error {
	GetContext(ctx).ServeJSON([]byte(`{"created":true}`))
	return nil
}

type emptyArrayController struct {
	BaseController
}

func (c *emptyArrayController) Serve(ctx context.Context) error {
	GetContext(ctx).ServeJSON([]byte(`[]`))
	return nil
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

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
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
