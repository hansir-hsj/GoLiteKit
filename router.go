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

	ctx = WithContext(ctx)
	gcx := GetContext(ctx)
	gcx.SetRequest(req)
	gcx.SetResponseWriter(w)

	path := req.URL.Path
	controller, ok := r.routers[path]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	controller.Serve(ctx)
}
