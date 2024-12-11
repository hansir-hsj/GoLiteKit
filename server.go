package datong

import (
	"net"
	"net/http"
)

type Server struct {
	context    Context
	addr       string
	httpServer http.Server
	handler    http.HandlerFunc
}

func New(addr string) *Server {
	return &Server{
		addr:       addr,
		httpServer: http.Server{},
	}
}

func (s *Server) Start() error {
	l, err := net.Listen("tcp4", s.addr)
	if err != nil {
		return err
	}
	s.httpServer.Handler = http.HandlerFunc(Handle)

	s.httpServer.Serve(l)
	return nil
}

func Handle(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello world!"))
}
