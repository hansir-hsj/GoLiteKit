package golite

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

type Router struct {
	routers     map[string]Controller
	wildRouters *Trie
}

func NewRouter() Router {
	return Router{
		routers:     make(map[string]Controller),
		wildRouters: NewTrie(),
	}
}

func (r *Router) Register(path string, controller Controller) {
	if strings.Contains(path, ":") {
		r.wildRouters.Add(path, controller)
		return
	}
	r.routers[path] = controller
}

func (r *Router) Route(path string) (Controller, bool) {
	if strings.Contains(path, ":") {
		return r.wildRouters.Get(path)
	}
	if controller, ok := r.routers[path]; ok {
		return controller, true
	}
	return nil, false
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	ctx = WithContext(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req), WithResponseWriter(w))

	controller, ok := r.Route(req.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	doneChan := make(chan struct{}, 1)
	panicChan := make(chan any, 1)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				panicChan <- p
			}
		}()
		controller.Serve(ctx)
		doneChan <- struct{}{}
	}()

	select {
	case p := <-panicChan:
		log.Printf("%v", p)
	case <-ctx.Done():
		log.Print("timeout")
	case <-doneChan:
	}
}
