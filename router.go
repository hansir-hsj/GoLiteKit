package golitekit

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/hansir-hsj/GoLiteKit/logger"
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
	// Per request, reflect.New allocates a fresh zero-value instance — no deep copy needed.
	t := reflect.TypeOf(c).Elem()

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := newContext(req)
		ctx = logger.WithLoggerContext(ctx)
		req = req.WithContext(ctx)

		gcx := GetContext(ctx)
		gcx.SetContextOptions(
			WithRequest(req),
			WithResponseWriter(w),
			withServices(r.services),
		)

		controllerHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			handler := reflect.New(t).Interface().(Controller)

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

		var h http.Handler = controllerHandler
		if len(groupMiddlewares) > 0 {
			h = groupMiddlewares.Apply(h)
		}
		h = r.middlewares.Apply(h)
		h.ServeHTTP(w, req)
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
