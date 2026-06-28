package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// HandlerFunc is a lightweight handler that receives the request Context directly.
type HandlerFunc func(ctx *Context) error

type routeTarget struct {
	controller Controller
	handler    HandlerFunc
}

func newRouteTarget(c any) routeTarget {
	switch h := c.(type) {
	case Controller:
		return routeTarget{controller: h}
	case HandlerFunc:
		return routeTarget{handler: h}
	case func(*Context) error:
		return routeTarget{handler: HandlerFunc(h)}
	default:
		if isControllerValue(c) {
			panic(fmt.Sprintf("golitekit: controller must be a pointer to struct, got %T", c))
		}
		panic(fmt.Sprintf("golitekit: unsupported handler type %T", c))
	}
}

func isControllerValue(c any) bool {
	if c == nil {
		return false
	}
	t := reflect.TypeOf(c)
	if t.Kind() != reflect.Struct {
		return false
	}
	ptrType := reflect.PointerTo(t)
	return ptrType.Implements(reflect.TypeOf((*Controller)(nil)).Elem())
}

// Router handles route registration and middleware.
type Router struct {
	mux              *http.ServeMux
	registeredPaths  map[string]struct{} // tracks paths that already have a 405 catch-all
	middlewares      MiddlewareQueue
	services         *Services
	routesRegistered bool
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
	if r.routesRegistered {
		panic("golitekit: middleware must be registered before routes")
	}
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
	r.PATCH(path, c)
	r.HEAD(path, c)
	r.OPTIONS(path, c)
}

func (r *Router) handle(method, path string, c any, groupMiddlewares MiddlewareQueue) {
	r.routesRegistered = true
	target := newRouteTarget(c)
	handler := r.wrapRouteTarget(target, groupMiddlewares)

	// Register the method-specific handler directly (Go 1.22+ pattern syntax).
	r.mux.Handle(method+" "+path, handler)

	// Register a path-only catch-all once per path to return a JSON 405.
	if _, exists := r.registeredPaths[path]; !exists {
		r.registeredPaths[path] = struct{}{}
		appErr := ErrMethodNotAllowed("Method Not Allowed", nil)
		body, _ := json.Marshal(Response{Status: appErr.Code, Msg: appErr.Message})
		r.mux.Handle(path, r.wrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(appErr.Code)
			_, _ = w.Write(body)
		})))
	}
}

func (r *Router) wrapRouteTarget(target routeTarget, groupMiddlewares MiddlewareQueue) http.Handler {
	if target.handler != nil {
		return r.wrapContextHandler(target.handler, groupMiddlewares)
	}
	return r.wrapController(target.controller, groupMiddlewares)
}

func (r *Router) wrapContextHandler(fn HandlerFunc, groupMiddlewares MiddlewareQueue) http.Handler {
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		gcx := GetContext(ctx)
		if gcx == nil {
			return fmt.Errorf("golitekit: context not initialized")
		}
		gcx.setContextOptions(withRequest(req), withResponseWriter(w))
		return fn(gcx)
	})

	var prebuilt Handler = innerHandler
	if len(groupMiddlewares) > 0 {
		prebuilt = groupMiddlewares.Apply(prebuilt)
	}
	prebuilt = r.middlewares.Apply(prebuilt)

	return r.wrapHandlerWithContext(prebuilt)
}

func (r *Router) wrapController(c Controller, groupMiddlewares MiddlewareQueue) http.Handler {
	// Extract the concrete type once at registration time.
	ctrlType := reflect.TypeOf(c)
	if ctrlType.Kind() != reflect.Pointer || ctrlType.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("golitekit: controller must be a pointer to struct, got %T", c))
	}
	t := ctrlType.Elem()
	prototype := reflect.ValueOf(c).Elem()

	newController := func() Controller {
		v := reflect.New(t)
		v.Elem().Set(prototype)
		return v.Interface().(Controller)
	}

	// innerHandler is stable: built once at registration, not recreated per request.
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		if gcx := GetContext(ctx); gcx != nil {
			gcx.setContextOptions(withRequest(req), withResponseWriter(w))
		}

		handler := newController()

		// Call optional lifecycle hooks if implemented
		if init, ok := handler.(Initializer); ok {
			if err := init.Init(ctx); err != nil {
				return WrapError(err, http.StatusInternalServerError)
			}
		}
		// Parse before validation so Validate can inspect bound request data.
		// Custom RequestParser implementations own request parsing; the router does
		// not pre-read the request body. BaseControllerOf.ParseRequest handles the
		// default JSON/form/multipart parsing path.
		if parser, ok := handler.(RequestParser); ok {
			if err := parser.ParseRequest(ctx); err != nil {
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

	return r.wrapHandlerWithContext(prebuilt)
}

func (r *Router) wrapHTTPHandler(handler http.Handler) http.Handler {
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		handler.ServeHTTP(w, req)
		return nil
	})

	prebuilt := r.middlewares.Apply(innerHandler)
	return r.wrapHandlerWithContext(prebuilt)
}

func (r *Router) wrapHandlerWithContext(prebuilt Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		glkCtx := newContext(req)
		req = req.WithContext(glkCtx)

		gcx := &glkCtx.gcx
		gcx.setContextOptions(
			withRequest(req),
			withResponseWriter(w),
			withServices(r.services),
		)
		if r.services != nil && r.services.Observer() != nil {
			req = req.WithContext(WithObserverContext(req.Context(), r.services.Observer()))
			gcx.setContextOptions(withRequest(req))
		}

		// Errors propagate up through the middleware chain. ErrorHandlerMiddleware
		// (when present) handles them; otherwise fall back to a plain HTTP error.
		if err := prebuilt(req.Context(), w, req); err != nil {
			appErr := WrapError(err, http.StatusInternalServerError)
			http.Error(w, appErr.Message, appErr.Code)
		}
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
	r.routesRegistered = true
	r.mux.Handle(urlPath+"/", r.wrapHTTPHandler(http.StripPrefix(urlPath, fs)))
}

// Handler returns the http.Handler.
func (r *Router) Handler() http.Handler { return r.mux }
