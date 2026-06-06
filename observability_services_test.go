package golitekit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type recordingObserver struct {
	started []string
}

func (o *recordingObserver) StartSpan(ctx context.Context, name string, attrs ...Attribute) (context.Context, Span) {
	o.started = append(o.started, name)
	return ctx, noopSpan{}
}

type orderLogger struct {
	order *[]string
}

func (l orderLogger) Debug(ctx context.Context, msg string, args ...any)   {}
func (l orderLogger) Trace(ctx context.Context, msg string, args ...any)   {}
func (l orderLogger) Warning(ctx context.Context, msg string, args ...any) {}
func (l orderLogger) Error(ctx context.Context, msg string, args ...any)   {}
func (l orderLogger) Fatal(ctx context.Context, msg string, args ...any)   {}
func (l orderLogger) Close() error                                         { return nil }
func (l orderLogger) Info(ctx context.Context, msg string, args ...any) {
	*l.order = append(*l.order, "logger")
}

type captureStatusWriter struct {
	http.ResponseWriter
	statusCode int
	wrote      bool
}

func (w *captureStatusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.wrote = true
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *captureStatusWriter) WriteHeader(statusCode int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func TestWithObserverStoresObserver(t *testing.T) {
	observer := &recordingObserver{}
	app := NewApp(WithObserver(observer))

	if got := app.Services.Observer(); got != observer {
		t.Fatalf("Services.Observer() = %v, want %v", got, observer)
	}
}

func TestWithObservabilityMiddlewareIsInsertedBeforeLogger(t *testing.T) {
	var order []string
	observabilityMiddleware := Middleware(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			order = append(order, "observability-before")
			err := next(ctx, w, r)
			order = append(order, "observability-after")
			return err
		}
	})

	app := NewApp(
		WithLogger(orderLogger{order: &order}),
		WithObservabilityMiddleware(observabilityMiddleware),
	)
	app.GET("/order", HandlerFunc(func(ctx *Context) error {
		order = append(order, "handler")
		return ctx.String(http.StatusOK, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/order", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := []string{"observability-before", "handler", "logger", "observability-after"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestObservabilityMiddlewareSeesHandledErrorStatus(t *testing.T) {
	var observedStatus int
	observabilityMiddleware := Middleware(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			cw := &captureStatusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			err := next(ctx, cw, r)
			observedStatus = cw.statusCode
			return err
		}
	})

	app := NewApp(WithObservabilityMiddleware(observabilityMiddleware))
	app.GET("/bad", HandlerFunc(func(ctx *Context) error {
		return ErrBadRequest("bad request", nil)
	}))

	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if observedStatus != http.StatusBadRequest {
		t.Fatalf("observed status = %d, want %d", observedStatus, http.StatusBadRequest)
	}
}

func TestAppObserverAvailableInHandler(t *testing.T) {
	observer := &recordingObserver{}
	app := NewApp(WithObserver(observer))
	app.GET("/span", HandlerFunc(func(ctx *Context) error {
		_, span := StartSpan(ctx.Request().Context(), "handler-span")
		span.End()
		return ctx.String(http.StatusOK, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/span", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !reflect.DeepEqual(observer.started, []string{"handler-span"}) {
		t.Fatalf("started spans = %v, want [handler-span]", observer.started)
	}
}
