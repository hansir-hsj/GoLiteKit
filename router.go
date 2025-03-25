package golitekit

import (
	"net/http"
	"strings"
)

type Router struct {
	// method -> path -> controller
	static      map[string]Controller
	routers     map[string]map[string]Controller
	wildRouters map[string]*Trie
}

func NewRouter() *Router {
	return &Router{
		static:      make(map[string]Controller),
		routers:     make(map[string]map[string]Controller, 4),
		wildRouters: make(map[string]*Trie, 4),
	}
}

func (r *Router) OnPost(path string, controller Controller) {
	path = dealSlash(path)
	r.register(http.MethodPost, path, controller)
}

func (r *Router) OnGet(path string, controller Controller) {
	path = dealSlash(path)
	r.register(http.MethodGet, path, controller)
}

func (r *Router) OnPut(path string, controller Controller) {
	path = dealSlash(path)
	r.register(http.MethodPut, path, controller)
}

func (r *Router) OnDelete(path string, controller Controller) {
	path = dealSlash(path)
	r.register(http.MethodDelete, path, controller)
}

func (r *Router) Static(path string, controller Controller) {
	path = dealSlash(path)
	r.static[path] = controller
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

func dealSlash(path string) string {
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	path = strings.TrimRight(path, "/")
	return path
}

func (r *Router) Route(method, path string) (Controller, map[string]string, bool) {
	path = dealSlash(path)

	// match regular routes first
	if router, ok := r.routers[method]; ok {
		if controller, ok := router[path]; ok {
			return controller, nil, true
		}
	}

	// re match wild routes
	if trie, ok := r.wildRouters[method]; ok {
		return trie.Get(path)
	}

	// finally match static routes
	if method == http.MethodGet && strings.HasPrefix(path, "/") {
		if controller, ok := r.static[path]; ok {
			return controller, nil, true
		}
	}
	return nil, nil, false
}
