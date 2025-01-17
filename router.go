package golite

import (
	"net/http"
	"strings"
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
