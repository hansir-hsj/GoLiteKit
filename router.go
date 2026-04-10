package golitekit

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sync"
)

// Router handles route registration and middleware.
type Router struct {
	mux            *http.ServeMux
	methodHandlers map[string]map[string]http.Handler // path -> method -> handler
	middlewares    MiddlewareQueue
	services       *Services
}

// NewRouter creates a new Router.
func NewRouter(services *Services) *Router {
	return &Router{
		mux:            http.NewServeMux(),
		methodHandlers: make(map[string]map[string]http.Handler),
		middlewares:    NewMiddlewareQueue(),
		services:       services,
	}
}

// Use adds global middlewares.
func (r *Router) Use(middlewares ...HandlerMiddleware) *Router {
	r.middlewares.Use(middlewares...)
	return r
}

func (r *Router) GET(path string, c Controller)     { r.handle(http.MethodGet, path, c, nil) }
func (r *Router) POST(path string, c Controller)    { r.handle(http.MethodPost, path, c, nil) }
func (r *Router) PUT(path string, c Controller)     { r.handle(http.MethodPut, path, c, nil) }
func (r *Router) DELETE(path string, c Controller)  { r.handle(http.MethodDelete, path, c, nil) }
func (r *Router) PATCH(path string, c Controller)   { r.handle(http.MethodPatch, path, c, nil) }
func (r *Router) HEAD(path string, c Controller)    { r.handle(http.MethodHead, path, c, nil) }
func (r *Router) OPTIONS(path string, c Controller) { r.handle(http.MethodOptions, path, c, nil) }

// Any registers all common HTTP methods.
func (r *Router) Any(path string, c Controller) {
	r.GET(path, c)
	r.POST(path, c)
	r.PUT(path, c)
	r.DELETE(path, c)
}

func (r *Router) handle(method, path string, c Controller, groupMiddlewares MiddlewareQueue) {
	handler := r.wrapController(c, groupMiddlewares)

	if r.methodHandlers[path] == nil {
		r.methodHandlers[path] = make(map[string]http.Handler)
		r.mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
			if h, ok := r.methodHandlers[path][req.Method]; ok {
				h.ServeHTTP(w, req)
			} else {
				appErr := ErrMethodNotAllowed("Method Not Allowed")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(appErr.Code)
				json.NewEncoder(w).Encode(Response{Status: appErr.Code, Msg: appErr.Message})
			}
		})
	}

	r.methodHandlers[path][method] = handler
}

func (r *Router) wrapController(c Controller, groupMiddlewares MiddlewareQueue) http.Handler {
	// Extract the concrete type once at registration time.
	t := reflect.TypeOf(c).Elem()
	ctrlPool := &sync.Pool{
		New: func() any { return reflect.New(t).Interface() },
	}

	// innerHandler is stable: built once at registration, not recreated per request.
	// It reads gcx from the request context so it captures no per-request state.
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		gcx := GetContext(ctx)
		handler := ctrlPool.Get().(Controller)
		defer func() {
			// Zero all fields and return to the pool to avoid GC churn.
			reflect.ValueOf(handler).Elem().Set(reflect.Zero(t))
			ctrlPool.Put(handler)
		}()

		if err := handler.Init(ctx); err != nil {
			if !HasError(ctx) {
				SetError(ctx, ErrInternal("Controller init failed", err))
			}
			return
		}
		if err := handler.SanityCheck(ctx); err != nil {
			if !HasError(ctx) {
				SetError(ctx, ErrBadRequest("Sanity check failed", err))
			}
			return
		}
		if err := handler.ParseRequest(ctx, gcx.RawBody); err != nil {
			if !HasError(ctx) {
				SetError(ctx, ErrBadRequest("Parse request failed", err))
			}
			return
		}
		if err := handler.Serve(ctx); err != nil {
			if !HasError(ctx) {
				SetError(ctx, ErrInternal("Controller serve failed", err))
			}
			return
		}
		if err := handler.Finalize(ctx); err != nil {
			if !HasError(ctx) {
				SetError(ctx, ErrInternal("Controller finalize failed", err))
			}
			return
		}
	})

	// Pre-apply the full middleware chain at registration time (not per-request).
	// This eliminates N closure allocations per request (one per middleware layer).
	// Requirement: all router.Use() and group.Use() calls must precede route registration.
	var prebuilt http.Handler = innerHandler
	if len(groupMiddlewares) > 0 {
		prebuilt = groupMiddlewares.Apply(prebuilt)
	}
	prebuilt = r.middlewares.Apply(prebuilt)

	// Per-request handler: lightweight context injection only.
	// glkContext is pooled and embeds both Context and LoggerContext by value,
	// eliminating two context.WithValue allocations per request.
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		glkCtx := newContext(req)
		req = req.WithContext(glkCtx)

		gcx := &glkCtx.gcx
		gcx.SetContextOptions(
			WithRequest(req),
			WithResponseWriter(w),
			withServices(r.services),
		)

		prebuilt.ServeHTTP(w, req)
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
