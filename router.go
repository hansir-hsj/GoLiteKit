package golite

import (
	"context"
	"github/hsj/golite/env"
	"github/hsj/golite/logger"
	"log"
	"net/http"
	"strings"
	"time"
)

type Router struct {
	// method -> path -> controller
	routers     map[string]map[string]Controller
	wildRouters map[string]*Trie
}

func NewRouter() Router {
	return Router{
		routers:     make(map[string]map[string]Controller, 4),
		wildRouters: make(map[string]*Trie, 4),
	}
}

func (r *Router) OnPost(path string, controller Controller) {
	r.register(http.MethodPost, path, controller)
}

func (r *Router) OnGet(path string, controller Controller) {
	r.register(http.MethodGet, path, controller)
}

func (r *Router) OnPut(path string, controller Controller) {
	r.register(http.MethodPut, path, controller)
}

func (r *Router) OnDelete(path string, controller Controller) {
	r.register(http.MethodDelete, path, controller)
}

func (r *Router) register(method, path string, controller Controller) {
	if strings.Contains(path, ":") {
		if _, ok := r.wildRouters[method]; !ok {
			r.wildRouters[method] = NewTrie()
		}
		r.wildRouters[method].Add(path, controller)
	} else {
		if _, ok := r.routers[method]; !ok {
			r.routers[method] = make(map[string]Controller)
		}
		r.routers[method][path] = controller
	}
}

func (r *Router) Route(method, path string) (Controller, map[string]string, bool) {
	// 先匹配普通路由
	if router, ok := r.routers[method]; ok {
		if controller, ok := router[path]; ok {
			return controller, nil, true
		}
	}
	// 再匹配带参数的路由
	if trie, ok := r.wildRouters[method]; ok {
		return trie.Get(path)
	}
	return nil, nil, false
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := logger.WithContext(req.Context())

	logInst, err := logger.NewLogger(ctx, env.ConfDir())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	panicLogInst, err := logger.NewPanicLogger(ctx, env.ConfDir())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx = WithContext(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req), WithResponseWriter(w), WithLogger(logInst), WithPanicLogger(panicLogInst))

	logger.AddInfo(ctx, "method", gcx.request.Method)
	logger.AddInfo(ctx, "url", gcx.request.URL)

	controller, params, ok := r.Route(req.Method, req.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if params != nil {
		gcx.SetContextOptions(WithRouterParams(params))
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	doneChan := make(chan struct{}, 1)
	panicChan := make(chan any, 1)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				panicLogInst.Report(ctx, p)
				panicChan <- p
			}
		}()
		ctx = WithTracker(ctx)
		tracker := GetTracker(ctx)
		gcx = GetContext(ctx)
		logInst := gcx.Logger()

		err := controller.Init(ctx)
		if err != nil {
			return
		}
		err = controller.Serve(ctx)
		if err != nil {
			return
		}
		err = controller.Finalize(ctx)
		if err != nil {
			return
		}
		tracker.LogTracker(ctx)
		logInst.Info(ctx, "ok")

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
