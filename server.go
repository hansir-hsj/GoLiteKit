package golite

import (
	"github/hsj/golite/env"
	"net/http"
)

type Server struct {
	addr            string
	router          Router
	middlewareQueue MiddlewareQueue
}

func New(conf string) *Server {
	router := NewRouter()

	if err := env.Init(conf); err != nil {
		return nil
	}

	return &Server{
		addr:            env.Addr(),
		router:          router,
		middlewareQueue: NewMiddlewareQueue(),
	}
}

func (s *Server) Start() error {
	server := http.Server{
		Addr:         s.addr,
		ReadTimeout:  env.ReadTimeout(),
		WriteTimeout: env.WriteTimeout(),
		IdleTimeout:  env.IdleTimeout(),
		Handler:      s,
	}
	s.Use(LoggerMiddleware, TrackerMiddleware, TimeoutMiddleware)

	return server.ListenAndServe()
}

func (s *Server) Use(middlewares ...Middleware) {
	s.middlewareQueue = append(s.middlewareQueue, middlewares...)
}

func (s *Server) OnGet(path string, controller Controller) {
	s.router.OnGet(path, controller)
}

func (s *Server) OnPost(path string, controller Controller) {
	s.router.OnPost(path, controller)
}

func (s *Server) OnPut(path string, controller Controller) {
	s.router.OnPut(path, controller)
}

func (s *Server) OnDelete(path string, controller Controller) {
	s.router.OnDelete(path, controller)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := WithContext(req.Context())
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req), WithResponseWriter(w))

	controller, params, ok := s.router.Route(req.Method, req.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if params != nil {
		gcx.SetContextOptions(WithRouterParams(params))
	}
	s.Use(ControllerAsMiddleware(ctx, controller, w, req))

	s.middlewareQueue.Next(ctx, w, req)
}
