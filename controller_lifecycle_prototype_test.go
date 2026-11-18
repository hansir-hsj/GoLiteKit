package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type tagsBody struct {
	Tags  map[string]string `json:"tags"`
	Limit int               `json:"limit"`
}

type tagsController struct {
	BaseControllerOf[tagsBody]
}

func (c *tagsController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, c.GetRequest())
}

// A map preset on the prototype's Request used to be decoded into by every
// request concurrently ("fatal error: concurrent map writes").
func TestControllerPrototypeRequestIsNotShared(t *testing.T) {
	r := newTestRouter()
	r.POST("/tags", &tagsController{BaseControllerOf: BaseControllerOf[tagsBody]{
		Request: tagsBody{Tags: map[string]string{"preset": "x"}},
	}})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"tags":{"k%d":"v"}}`, i)
			req := httptest.NewRequest(http.MethodPost, "/tags", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.Handler().ServeHTTP(rec, req)

			want := fmt.Sprintf(`{"tags":{"k%d":"v"},"limit":0}`, i)
			if rec.Body.String() != want {
				t.Errorf("body = %s, want %s", rec.Body.String(), want)
			}
		}(i)
	}
	wg.Wait()
}

type defaultsController struct {
	BaseControllerOf[tagsBody]
}

func (c *defaultsController) Init(ctx context.Context) error {
	if err := c.BaseControllerOf.Init(ctx); err != nil {
		return err
	}
	c.Request.Limit = 10 // per-request default, kept unless the body overrides it
	return nil
}

func (c *defaultsController) Serve(ctx context.Context) error {
	return c.JSON(http.StatusOK, c.GetRequest().Limit)
}

func TestControllerRequestDefaultsFromInitSurviveParsing(t *testing.T) {
	r := newTestRouter()
	r.POST("/d", &defaultsController{})

	for body, want := range map[string]string{`{}`: "10", `{"limit":3}`: "3"} {
		req := httptest.NewRequest(http.MethodPost, "/d", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, req)
		if rec.Body.String() != want {
			t.Errorf("body %s: got %s, want %s", body, rec.Body.String(), want)
		}
	}
}

type counterController struct {
	BaseController
	seen map[string]int
}

func (c *counterController) Serve(ctx context.Context) error {
	c.seen["x"]++
	return c.String(http.StatusOK, fmt.Sprint(c.seen["x"]))
}

func TestControllerFactoryCreatesFreshStatePerRequest(t *testing.T) {
	r := newTestRouter()
	calls := 0
	r.GET("/factory", ControllerFactory(func() Controller {
		calls++
		return &counterController{seen: map[string]int{}}
	}))
	r.GET("/func", func() Controller { return &counterController{seen: map[string]int{}} })

	for _, path := range []string{"/factory", "/factory", "/func", "/func"} {
		rec := httptest.NewRecorder()
		r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Body.String() != "1" {
			t.Fatalf("%s body = %q, want fresh state (1)", path, rec.Body.String())
		}
	}
	if calls != 2 {
		t.Fatalf("factory called %d times, want once per request", calls)
	}
}

func TestControllerFactoryNilIsInternalError(t *testing.T) {
	r := newTestRouter()
	r.GET("/nil", ControllerFactory(func() Controller { return nil }))

	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nil", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
