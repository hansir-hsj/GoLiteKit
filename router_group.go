package golitekit

import "net/http"

type RouterGroup struct {
	prefix string
	server *Server
}

func (s *Server) NewRouterGroup(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix: prefix,
		server: s,
	}
}

func (rg *RouterGroup) OnAny(path string, controller Controller) {
	rg.registerHandler(http.MethodGet, path, controller)
	rg.registerHandler(http.MethodPost, path, controller)
	rg.registerHandler(http.MethodPut, path, controller)
	rg.registerHandler(http.MethodDelete, path, controller)
}

func (rg *RouterGroup) OnGet(path string, controller Controller) {
	rg.registerHandler(http.MethodGet, path, controller)
}

func (rg *RouterGroup) OnPost(path string, controller Controller) {
	rg.registerHandler(http.MethodPost, path, controller)
}

func (rg *RouterGroup) OnPut(path string, controller Controller) {
	rg.registerHandler(http.MethodPut, path, controller)
}

func (rg *RouterGroup) OnDelete(path string, controller Controller) {
	rg.registerHandler(http.MethodDelete, path, controller)
}

func (rg *RouterGroup) registerHandler(method, path string, controller Controller) {
	fullPath := rg.prefix + path
	rg.server.registerHandler(method, fullPath, controller)
}
