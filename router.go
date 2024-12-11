package golite

import (
	"context"
	"net/http"
	"time"
)

type Router struct {
	routers map[string]Controller
}

func NewRouter() Router {
	return Router{routers: make(map[string]Controller)}
}

func (r *Router) Register(path string, controller Controller) {
	r.routers[path] = controller
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	WithContext(ctx)

	path := req.URL.Path
	controller, ok := r.routers[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
	}
	controller.Serve(ctx)
}
