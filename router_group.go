package golitekit

import "net/http"

type RouterGroup struct {
	prefix string
	server *Server
}

func newRouterGroup(prefix string) *RouterGroup {
	prefix = dealSlash(prefix)
	return &RouterGroup{
		prefix: prefix,
	}
}

func (g *RouterGroup) OnGet(path string, controller Controller) {
	path = dealSlash(path)
	g.server.router.register(http.MethodGet, g.prefix+path, controller)
}

func (g *RouterGroup) OnPost(path string, controller Controller) {
	path = dealSlash(path)
	g.server.router.register(http.MethodPost, g.prefix+path, controller)
}

func (g *RouterGroup) OnPut(path string, controller Controller) {
	path = dealSlash(path)
	g.server.router.register(http.MethodPut, g.prefix+path, controller)
}

func (g *RouterGroup) OnDelete(path string, controller Controller) {
	path = dealSlash(path)
	g.server.router.register(http.MethodDelete, g.prefix+path, controller)
}
