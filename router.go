package golitekit

import (
	"net/http"

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
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	r.methodHandlers[path][method] = handler
}

func (r *Router) wrapController(c Controller, groupMiddlewares MiddlewareQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := newContext(w, req, r.services)
		ctx = logger.WithLoggerContext(ctx)
		req = req.WithContext(ctx)

		gcx := GetContext(ctx)
		gcx.SetContextOptions(
			WithRequest(req),
			WithResponseWriter(w),
			_WithServices(r.services),
		)

		controllerHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			cloned := CloneController(c)

			if err := cloned.Init(ctx); err != nil {
				SetError(ctx, ErrInternal("Controller init failed", err))
				return
			}
			if err := cloned.SanityCheck(ctx); err != nil {
				SetError(ctx, ErrBadRequest("Sanity check failed", err))
				return
			}
			if err := cloned.ParseRequest(ctx, gcx.RawBody); err != nil {
				SetError(ctx, ErrBadRequest("Parse request failed", err))
				return
			}
			if err := cloned.Serve(ctx); err != nil {
				SetError(ctx, ErrInternal("Controller serve failed", err))
				return
			}
			if err := cloned.Finalize(ctx); err != nil {
				SetError(ctx, ErrInternal("Controller finalize failed", err))
				return
			}
		})

		var handler http.Handler = controllerHandler
		if len(groupMiddlewares) > 0 {
			handler = groupMiddlewares.Apply(handler)
		}
		handler = r.middlewares.Apply(handler)
		handler.ServeHTTP(w, req)
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
