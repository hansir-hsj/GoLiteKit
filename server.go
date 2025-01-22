package golite

import (
	"context"
	"fmt"
	"github/hsj/golite/env"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type Server struct {
	addr            string
	router          Router
	middlewareQueue MiddlewareQueue
	rateLimiter     *RateLimiter

	httpServer http.Server
	closeChan  chan struct{}
}

func New(conf string) *Server {
	router := NewRouter()

	if err := env.Init(conf); err != nil {
		return nil
	}

	var rateLimiter *RateLimiter
	if env.RateLimit() > 0 {
		rateLimiter = NewRateLimiter(env.RateLimit(), env.RateBurst())
	}

	return &Server{
		addr:            env.Addr(),
		router:          router,
		middlewareQueue: NewMiddlewareQueue(),
		rateLimiter:     rateLimiter,
		closeChan:       make(chan struct{}),
	}
}

func (s *Server) Start() {
	s.httpServer = http.Server{
		Addr:         s.addr,
		ReadTimeout:  env.ReadTimeout(),
		WriteTimeout: env.WriteTimeout(),
		IdleTimeout:  env.IdleTimeout(),
		Handler:      s,
	}
	s.Use(LoggerMiddleware, TrackerMiddleware, TimeoutMiddleware)

	go s.handleSignal()

	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		fmt.Printf("server start error: %v", err)
	}
	<-s.closeChan
}

func (s *Server) handleSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	switch sig {
	case syscall.SIGINT:
		fmt.Println("server shutdown by SIGINT")
	case syscall.SIGTERM:
		fmt.Println("server shutdown by SIGTERM")
	}
	s.httpServer.Shutdown(context.Background())
	s.closeChan <- struct{}{}
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

	if s.rateLimiter != nil {
		s.Use(s.rateLimiter.RateLimiterAsMiddleware(ctx, w, req, s.middlewareQueue))
	}

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
