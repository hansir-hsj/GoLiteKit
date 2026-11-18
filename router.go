package golitekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// HandlerFunc is a lightweight handler that receives the request Context directly.
type HandlerFunc func(ctx *Context) error

// ControllerFactory creates a controller per request. Use it when the
// controller needs per-request state:
//
//	app.GET("/users", glk.ControllerFactory(func() glk.Controller {
//		return &UserController{repo: repo, seen: map[string]int{}}
//	}))
type ControllerFactory func() Controller

type routeTarget struct {
	controller Controller
	factory    ControllerFactory
	handler    HandlerFunc
}

func newRouteTarget(c any) routeTarget {
	switch h := c.(type) {
	case Controller:
		return routeTarget{controller: h}
	case ControllerFactory:
		return routeTarget{factory: h}
	case func() Controller:
		return routeTarget{factory: h}
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

// fallbackPattern matches every request no other route matches. It answers
// with a JSON 404, or a JSON 405 plus an Allow header when the path is
// registered for other methods.
const fallbackPattern = "/"

// probeMethods are the methods checked when building the Allow header.
var probeMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

// Router handles route registration and middleware. Paths use ServeMux
// syntax: "/" matches every unmatched path, "/{$}" only the root.
type Router struct {
	mux              *http.ServeMux
	outerMiddlewares MiddlewareQueue
	middlewares      MiddlewareQueue
	services         *Services
	routesRegistered bool

	fallbackOnce sync.Once
	fallback     http.Handler
}

// NewRouter creates a new Router.
func NewRouter(services *Services) *Router {
	r := &Router{
		mux:         http.NewServeMux(),
		middlewares: NewMiddlewareQueue(),
		services:    services,
	}
	r.mux.Handle(fallbackPattern, http.HandlerFunc(r.serveUnmatched))
	return r
}

// Use adds global middlewares. They run after middlewares added with UseOuter.
func (r *Router) Use(middlewares ...Middleware) *Router {
	if r.routesRegistered {
		panic("golitekit: middleware must be registered before routes")
	}
	r.middlewares.Use(middlewares...)
	return r
}

// UseOuter adds middlewares that wrap the default stack, so their headers
// (e.g. CORS) apply to error responses too. Their errors bypass the error
// handler and are written as plain text.
func (r *Router) UseOuter(middlewares ...Middleware) *Router {
	if r.routesRegistered {
		panic("golitekit: middleware must be registered before routes")
	}
	r.outerMiddlewares.Use(middlewares...)
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

	// Go 1.22+ pattern syntax. Requests for this path with other methods fall
	// through to the fallback, which answers 405 with an Allow header.
	r.mux.Handle(method+" "+path, handler)
}

// serveUnmatched handles requests no route matched. The chain is built on
// first use, so it includes every registered middleware.
func (r *Router) serveUnmatched(w http.ResponseWriter, req *http.Request) {
	r.fallbackOnce.Do(func() {
		r.fallback = r.wrapHTTPHandler(http.HandlerFunc(r.writeUnmatched))
	})
	// The fallback is not a route; don't report it as "/".
	unmatched := *req
	unmatched.Pattern = ""
	r.fallback.ServeHTTP(w, &unmatched)
}

func (r *Router) writeUnmatched(w http.ResponseWriter, req *http.Request) {
	code := http.StatusNotFound
	if allowed := r.allowedMethods(req); len(allowed) > 0 {
		code = http.StatusMethodNotAllowed
		w.Header().Set("Allow", strings.Join(allowed, ", "))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Response{
		Status: code,
		Msg:    http.StatusText(code),
		LogID:  EnsureLogID(req.Context()),
	})
}

// allowedMethods returns the methods for which a route other than the
// fallback matches req's path.
func (r *Router) allowedMethods(req *http.Request) []string {
	var allowed []string
	for _, method := range probeMethods {
		if method == req.Method {
			continue
		}
		probe := *req
		probe.Method = method
		if _, pattern := r.mux.Handler(&probe); pattern != "" && pattern != fallbackPattern {
			allowed = append(allowed, method)
		}
	}
	return allowed
}

func (r *Router) wrapRouteTarget(target routeTarget, groupMiddlewares MiddlewareQueue) http.Handler {
	switch {
	case target.handler != nil:
		return r.wrapContextHandler(target.handler, groupMiddlewares)
	case target.factory != nil:
		return r.wrapController(target.factory, groupMiddlewares)
	default:
		return r.wrapController(prototypeFactory(target.controller), groupMiddlewares)
	}
}

func (r *Router) wrapContextHandler(fn HandlerFunc, groupMiddlewares MiddlewareQueue) http.Handler {
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		gcx := GetContext(ctx)
		if gcx == nil {
			return fmt.Errorf("golitekit: context not initialized")
		}
		tw := &writeTracker{ResponseWriter: w}
		gcx.setContextOptions(withRequest(req), withResponseWriter(tw))
		if err := fn(gcx); err != nil {
			return err
		}
		return writeStoredResponse(gcx, w, tw)
	})

	return r.wrapHandlerWithContext(r.applyMiddlewares(innerHandler, groupMiddlewares))
}

// applyMiddlewares wraps h so that outer middlewares run first, then global,
// then group middlewares.
func (r *Router) applyMiddlewares(h Handler, groupMiddlewares MiddlewareQueue) Handler {
	if len(groupMiddlewares) > 0 {
		h = groupMiddlewares.Apply(h)
	}
	h = r.middlewares.Apply(h)
	return r.outerMiddlewares.Apply(h)
}

// prototypeFactory copies the registered controller for each request. Map,
// slice and pointer fields stay shared; the embedded Request is reset.
func prototypeFactory(c Controller) ControllerFactory {
	// Extract the concrete type once at registration time.
	ctrlType := reflect.TypeOf(c)
	if ctrlType.Kind() != reflect.Pointer || ctrlType.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("golitekit: controller must be a pointer to struct, got %T", c))
	}
	t := ctrlType.Elem()
	prototype := reflect.ValueOf(c).Elem()

	return func() Controller {
		v := reflect.New(t)
		v.Elem().Set(prototype)
		ctrl := v.Interface().(Controller)
		if rr, ok := ctrl.(requestResetter); ok {
			rr.resetRequest()
		}
		return ctrl
	}
}

func (r *Router) wrapController(newController ControllerFactory, groupMiddlewares MiddlewareQueue) http.Handler {
	// innerHandler is stable: built once at registration, not recreated per request.
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		// The stored response is written here, inside all middlewares, so
		// ones that wrap the ResponseWriter (e.g. compression) see it.
		tw := &writeTracker{ResponseWriter: w}
		gcx := GetContext(ctx)
		if gcx != nil {
			gcx.setContextOptions(withRequest(req), withResponseWriter(tw))
		}

		handler := newController()
		if handler == nil {
			return ErrInternal("Internal Server Error", errors.New("golitekit: controller factory returned nil"))
		}

		// Call optional lifecycle hooks if implemented
		if init, ok := handler.(Initializer); ok {
			if err := init.Init(ctx); err != nil {
				return WrapError(err, http.StatusInternalServerError)
			}
		}
		// Parse before Validate so it can inspect the bound request.
		if parser, ok := handler.(RequestParser); ok {
			if err := parser.ParseRequest(ctx); err != nil {
				return requestParseError(err)
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
		return writeStoredResponse(gcx, w, tw)
	})

	// Pre-apply the full middleware chain at registration time (not per-request).
	return r.wrapHandlerWithContext(r.applyMiddlewares(innerHandler, groupMiddlewares))
}

func (r *Router) wrapHTTPHandler(handler http.Handler) http.Handler {
	innerHandler := Handler(func(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
		handler.ServeHTTP(w, req)
		return nil
	})

	return r.wrapHandlerWithContext(r.applyMiddlewares(innerHandler, nil))
}

func (r *Router) wrapHandlerWithContext(prebuilt Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		glkCtx := newContext(req)
		if r.services != nil {
			glkCtx.observer = r.services.Observer()
		}
		req = req.WithContext(glkCtx)

		gcx := &glkCtx.gcx
		gcx.setContextOptions(
			withRequest(req),
			withResponseWriter(w),
			withServices(r.services),
		)

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

// Static serves files from fsPath under urlPath for GET and HEAD requests.
func (r *Router) Static(urlPath, fsPath string) {
	prefix := strings.TrimRight(urlPath, "/")
	fs := http.FileServer(http.Dir(fsPath))
	r.routesRegistered = true
	r.mux.Handle(http.MethodGet+" "+prefix+"/", r.wrapHTTPHandler(http.StripPrefix(prefix, fs)))
}

// Handler returns the http.Handler.
func (r *Router) Handler() http.Handler { return r.mux }
