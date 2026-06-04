package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
)

// HandlerFunc is a lightweight handler that receives the request Context directly.
type HandlerFunc func(ctx *Context) error

// handlerFuncController adapts a HandlerFunc to the Controller interface.
type handlerFuncController struct {
	BaseController
	fn HandlerFunc
}

func (c *handlerFuncController) Serve(ctx context.Context) error {
	gcx := GetContext(ctx)
	return c.fn(gcx)
}

// toController converts the handler argument to a Controller.
// Accepts: Controller, HandlerFunc, or func(*Context) error.
func toController(c any) Controller {
	switch h := c.(type) {
	case Controller:
		return h
	case HandlerFunc:
		return &handlerFuncController{fn: h}
	case func(*Context) error:
		return &handlerFuncController{fn: HandlerFunc(h)}
	default:
		panic(fmt.Sprintf("golitekit: unsupported handler type %T", c))
	}
}

// Router handles route registration and middleware.
type Router struct {
	mux             *http.ServeMux
	registeredPaths map[string]struct{} // tracks paths that already have a 405 catch-all
	middlewares     MiddlewareQueue
	services        *Services
}

// NewRouter creates a new Router.
func NewRouter(services *Services) *Router {
	return &Router{
		mux:             http.NewServeMux(),
		registeredPaths: make(map[string]struct{}),
		middlewares:     NewMiddlewareQueue(),
		services:        services,
	}
}

// Use adds global middlewares.
func (r *Router) Use(middlewares ...Middleware) *Router {
	r.middlewares.Use(middlewares...)
	return r
}

func (r *Router) GET(path string, c any)     { r.handle(http.MethodGet, path, c, nil) }
func (r *Router) POST(path string, c any)    { r.handle(http.MethodPost, path, c, nil) }
func (r *Router) PUT(path string, c any)     { r.handle(http.MethodPut, path, c, nil) }
func (r *Router) DELETE(path string, c any)  { r.handle(http.MethodDelete, path, c, nil) }
func (r *Router) PATCH(path string, c any)   { r.handle(http.MethodPatch, path, c, nil) }
func (r *Router) HEAD(path string, c any)    { r.handle(http.MethodHead, path, c, nil) }
func (r *Router) OPTIONS(path string, c any) { r.handle(http.MethodOptions, path, c, nil) }

// Any registers all common HTTP methods.
func (r *Router) Any(path string, c any) {
	r.GET(path, c)
	r.POST(path, c)
	r.PUT(path, c)
	r.DELETE(path, c)
}

func (r *Router) handle(method, path string, c any, groupMiddlewares MiddlewareQueue) {
	ctrl := toController(c)
	handler := r.wrapController(ctrl, groupMiddlewares)

	// Register the method-specific handler directly (Go 1.22+ pattern syntax).
	r.mux.Handle(method+" "+path, handler)

	// Register a path-only catch-all once per path to return a JSON 405.
	if _, exists := r.registeredPaths[path]; !exists {
		r.registeredPaths[path] = struct{}{}
		appErr := ErrMethodNotAllowed("Method Not Allowed")
		body, _ := json.Marshal(Response{Status: appErr.Code, Msg: appErr.Message})
		r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(appErr.Code)
			_, _ = w.Write(body)
		})
	}
}

func (r *Router) wrapController(c Controller, groupMiddlewares MiddlewareQueue) http.Handler {
	// Extract the concrete type once at registration time.
	t := reflect.TypeOf(c).Elem()
	prototype := reflect.ValueOf(c).Elem()
	ctrlPool := &sync.Pool{
		New: func() any {
			v := reflect.New(t)
			v.Elem().Set(prototype)
			return v.Interface()
		},
	}

	// innerHandler is stable: built once at registration, not recreated per request.
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		gcx := GetContext(ctx)
		handler := ctrlPool.Get().(Controller)
		defer func() {
			if res, ok := handler.(Resettable); ok {
				res.ResetController()
			} else {
				reflect.ValueOf(handler).Elem().Set(reflect.Zero(t))
			}
			ctrlPool.Put(handler)
		}()

		// Call optional lifecycle hooks if implemented
		if init, ok := handler.(Initializer); ok {
			if err := init.Init(ctx); err != nil {
				return WrapError(err, http.StatusInternalServerError)
			}
		}
		// Parse before validation so Validate can inspect bound request data.
		if parser, ok := handler.(RequestParser); ok {
			if err := parser.ParseRequest(ctx, gcx.RawBody); err != nil {
				return WrapError(err, http.StatusBadRequest)
			}
		}
		if val, ok := handler.(Validator); ok {
			if err := val.Validate(ctx); err != nil {
				return WrapError(err, http.StatusBadRequest)
			}
		}
		if err := handler.Serve(ctx); err != nil {
			return WrapError(err, http.StatusInternalServerError)
		}
		if fin, ok := handler.(Finalizer); ok {
			if err := fin.Finalize(ctx); err != nil {
				return WrapError(err, http.StatusInternalServerError)
			}
		}
		return nil
	})

	// Pre-apply the full middleware chain at registration time (not per-request).
	var prebuilt Handler = innerHandler
	if len(groupMiddlewares) > 0 {
		prebuilt = groupMiddlewares.Apply(prebuilt)
	}
	prebuilt = r.middlewares.Apply(prebuilt)

	// Per-request handler: lightweight context injection only.
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		glkCtx := newContext(req)
		req = req.WithContext(glkCtx)

		gcx := &glkCtx.gcx
		gcx.SetContextOptions(
			WithRequest(req),
			WithResponseWriter(w),
			withServices(r.services),
		)

		// Errors propagate up through the middleware chain. ErrorHandlerMiddleware
		// (when present) handles them; otherwise fall back to a plain HTTP error.
		if err := prebuilt(req.Context(), w, req); err != nil {
			appErr := WrapError(err, http.StatusInternalServerError)
			http.Error(w, appErr.Message, appErr.Code)
		}

		glkCtx.release()
	})
}

// Group creates a route group with prefix.
func (r *Router) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		router:      r,
		prefix:      prefix,
		middlewares: NewMiddlewareQueue(),
	}
}

// Static serves static files.
func (r *Router) Static(urlPath, fsPath string) {
	fs := http.FileServer(http.Dir(fsPath))
	r.mux.Handle(urlPath+"/", http.StripPrefix(urlPath, fs))
}

// Handler returns the http.Handler.
func (r *Router) Handler() http.Handler { return r.mux }

// Services returns the service container.
func (r *Router) Services() *Services { return r.services }
