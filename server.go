package golite

import (
	"net"
	"net/http"
)

type Server struct {
	addr       string
	httpServer http.Server
	router     Router
}

func New(addr string) *Server {
	router := NewRouter()
	return &Server{
		addr: addr,
		httpServer: http.Server{
			Handler: &router,
		},
		router: router,
	}
}

func (s *Server) Start() error {
	l, err := net.Listen("tcp4", s.addr)
	if err != nil {
		return err
	}
	s.httpServer.Serve(l)
	return nil
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
