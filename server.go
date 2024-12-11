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
	s.httpServer.Handler = &s.router

	s.httpServer.Serve(l)
	return nil
}
