package golite

import (
	"github/hsj/golite/env"
	"net/http"
)

type Server struct {
	addr   string
	router Router
}

func New(conf string) *Server {
	router := NewRouter()

	if err := env.Init(conf); err != nil {
		return nil
	}

	return &Server{
		addr:   env.Addr(),
		router: router,
	}
}

func (s *Server) Start() error {
	server := http.Server{
		Addr:         s.addr,
		Handler:      &s.router,
		ReadTimeout:  env.ReadTimeout(),
		WriteTimeout: env.WriteTimeout(),
		IdleTimeout:  env.IdleTimeout(),
	}
	return server.ListenAndServe()
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
