package golitekit

import "net/http"

// RouterGroup is a group of routes with shared prefix and middlewares.
type RouterGroup struct {
	router      *Router
	prefix      string
	middlewares MiddlewareQueue
}

// Use adds middlewares to this group.
func (g *RouterGroup) Use(middlewares ...HandlerMiddleware) *RouterGroup {
	g.middlewares.Use(middlewares...)
	return g
}

func (g *RouterGroup) GET(path string, c Controller)     { g.handle(http.MethodGet, path, c) }
func (g *RouterGroup) POST(path string, c Controller)    { g.handle(http.MethodPost, path, c) }
func (g *RouterGroup) PUT(path string, c Controller)     { g.handle(http.MethodPut, path, c) }
func (g *RouterGroup) DELETE(path string, c Controller)  { g.handle(http.MethodDelete, path, c) }
func (g *RouterGroup) PATCH(path string, c Controller)   { g.handle(http.MethodPatch, path, c) }
func (g *RouterGroup) HEAD(path string, c Controller)    { g.handle(http.MethodHead, path, c) }
func (g *RouterGroup) OPTIONS(path string, c Controller) { g.handle(http.MethodOptions, path, c) }

func (g *RouterGroup) Any(path string, c Controller) {
	g.GET(path, c)
	g.POST(path, c)
	g.PUT(path, c)
	g.DELETE(path, c)
}

func (g *RouterGroup) handle(method, path string, c Controller) {
	g.router.handle(method, g.prefix+path, c, g.middlewares)
}

// Group creates a nested group inheriting parent middlewares.
func (g *RouterGroup) Group(prefix string) *RouterGroup {
	return &RouterGroup{
		router:      g.router,
		prefix:      g.prefix + prefix,
		middlewares: g.middlewares.Clone(),
	}
}
