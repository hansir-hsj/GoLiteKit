package golitekit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

type Server struct {
	network string
	addr    string

	mux        *http.ServeMux
	listener   net.Listener
	httpServer http.Server

	logger      logger.Logger
	panicLogger *logger.PanicLogger
	rateLimiter *RateLimiter
	mq          MiddlewareQueue

	closeChan chan struct{}
}

func New(conf string) *Server {
	mux := http.NewServeMux()

	if err := env.Init(conf); err != nil {
		fmt.Fprintf(os.Stderr, "env init error: %v", err)
		return nil
	}

	var rateLimiter *RateLimiter
	if env.RateLimit() > 0 {
		rateLimiter = NewRateLimiter(env.RateLimit(), env.RateBurst())
	}

	logInst, err := logger.NewLogger(env.LoggerConfigFile())
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v", err)
		return nil
	}
	panicLogger, err := logger.NewPanicLogger(env.LoggerConfigFile())
	if err != nil {
		fmt.Fprintf(os.Stderr, "panic logger init error: %v", err)
		return nil
	}

	// inner middleware
	mq := NewMiddlewareQueue()
	mq.Use(LoggerAsMiddleware(logInst, panicLogger), TrackerMiddleware, ContextAsMiddleware(), TimeoutMiddleware)
	if rateLimiter != nil {
		mq.Use(rateLimiter.RateLimiterAsMiddleware())
	}

	return &Server{
		network:     env.Network(),
		addr:        env.Addr(),
		mux:         mux,
		rateLimiter: rateLimiter,
		closeChan:   make(chan struct{}),
		mq:          mq,
		logger:      logInst,
		panicLogger: panicLogger,
	}
}

func (s *Server) Start() error {
	s.httpServer = http.Server{
		ReadTimeout:    env.ReadTimeout(),
		WriteTimeout:   env.WriteTimeout(),
		IdleTimeout:    env.IdleTimeout(),
		MaxHeaderBytes: env.MaxHeaderBytes(),
		Handler:        s.mux,
	}

	if env.ReadHeaderTimeout() > 0 {
		s.httpServer.ReadHeaderTimeout = env.ReadHeaderTimeout()
	}

	go s.handleSignal()

	l, err := net.Listen(s.network, s.addr)
	if err != nil {
		return fmt.Errorf("listen error: %v", err)
	}

	if env.TLS() {
		certFile := env.TLSCertFile()
		keyFile := env.TLSKeyFile()
		if certFile == "" || keyFile == "" {
			return fmt.Errorf("TLS is enabled but certFile or keyFile is not set")
		}
		cer, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
		config := &tls.Config{Certificates: []tls.Certificate{cer}}
		l = tls.NewListener(l, config)
	}

	s.listener = l
	err = s.httpServer.Serve(l)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server start error: %s", err)
	}

	<-s.closeChan

	return nil
}

func (s *Server) handleSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	switch sig {
	case syscall.SIGINT:
	case syscall.SIGTERM:
		fmt.Fprintf(os.Stderr, "%s receive signal %v\n", time.Now(), sig)
	}
	ctx, cancel := context.WithTimeout(context.Background(), env.ShutdownTimeout())
	defer cancel()

	s.httpServer.Shutdown(ctx)
	s.closeChan <- struct{}{}
}

func (s *Server) OnAny(path string, controller Controller) {
	s.registerHandler(http.MethodGet, path, controller)
	s.registerHandler(http.MethodPost, path, controller)
	s.registerHandler(http.MethodPut, path, controller)
	s.registerHandler(http.MethodDelete, path, controller)
}

func (s *Server) OnGet(path string, controller Controller) {
	s.registerHandler(http.MethodGet, path, controller)
}

func (s *Server) OnPost(path string, controller Controller) {
	s.registerHandler(http.MethodPost, path, controller)
}

func (s *Server) OnPut(path string, controller Controller) {
	s.registerHandler(http.MethodPut, path, controller)
}

func (s *Server) OnDelete(path string, controller Controller) {
	s.registerHandler(http.MethodDelete, path, controller)
}

func (s *Server) registerHandler(method, path string, controller Controller) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		ctx := WithContext(r.Context())
		ctx = logger.WithLoggerContext(ctx)
		gcx := GetContext(ctx)
		gcx.SetContextOptions(WithRequest(r), WithResponseWriter(w))

		mq := s.mq.Clone()

		cloned := CloneController(controller)
		mq.Use(controllerAsMiddleware(cloned))
		mq.Next(ctx)
	}

	s.mux.HandleFunc(path, handler)
}

func (s *Server) UseMiddleware(middlewares ...Middleware) {
	s.mq.Use(middlewares...)
}
