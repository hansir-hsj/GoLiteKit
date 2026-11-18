package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type noopObserver struct{}

func (noopObserver) StartSpan(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span) {
	return ctx, noopSpan{}
}

func TestContextQueryCacheFollowsRawQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?a=1", nil)
	gcx := &Context{}
	gcx.setContextOptions(withRequest(req))

	if gcx.Query("a") != "1" {
		t.Fatalf("a = %q", gcx.Query("a"))
	}
	req.URL.RawQuery = "a=2" // e.g. rewritten by a middleware
	if gcx.Query("a") != "2" {
		t.Fatalf("a = %q after RawQuery change, want reparsed value", gcx.Query("a"))
	}
}

func TestObserverAvailableWithoutExtraContextLayer(t *testing.T) {
	obs := noopObserver{}
	app := NewApp(WithObserver(obs))
	var got Observer
	var direct bool
	app.GET("/", func(ctx *Context) error {
		reqCtx := ctx.Request().Context()
		got = ObserverFromContext(reqCtx)
		_, direct = reqCtx.(*glkContext)
		return nil
	})
	app.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got != obs {
		t.Fatalf("observer = %v, want the app observer", got)
	}
	if !direct {
		t.Fatal("request context should be the framework context itself, with no WithValue layer for the observer")
	}
}

// BenchmarkAppRequest measures the per-request cost of the default App stack.
func BenchmarkAppRequest(b *testing.B) {
	app := NewApp()
	app.GET("/users/{id}", func(ctx *Context) error {
		_ = ctx.Query("a")
		_ = ctx.Query("b")
		return ctx.String(http.StatusOK, ctx.Param("id"))
	})
	h := app.Handler()
	req := httptest.NewRequest(http.MethodGet, "/users/42?a=1&b=2", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkAppRequestWithObserver adds an Observer, which used to cost an
// extra context and request copy per request.
func BenchmarkAppRequestWithObserver(b *testing.B) {
	app := NewApp(WithObserver(noopObserver{}))
	app.GET("/users/{id}", func(ctx *Context) error {
		return ctx.String(http.StatusOK, ctx.Param("id"))
	})
	h := app.Handler()
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}
