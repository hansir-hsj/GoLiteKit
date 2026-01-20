package golitekit

import "net/http"

type RouterGroup struct {
	prefix      string
	server      *Server
	middlewares MiddlewareQueue
}

func (s *Server) NewRouterGroup(prefix string) *RouterGroup {
	return &RouterGroup{
		prefix:      prefix,
		server:      s,
		middlewares: NewMiddlewareQueue(),
	}
}

// Use adds middlewares to this router group.
// These middlewares will be executed after global middlewares
// and before the controller handler.
func (rg *RouterGroup) Use(middlewares ...HandlerMiddleware) *RouterGroup {
	rg.middlewares.Use(middlewares...)
	return rg
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
	rg.server.registerHandlerWithMiddlewares(method, fullPath, controller, rg.middlewares)
}
