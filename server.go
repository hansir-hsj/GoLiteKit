package golitekit

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

	mq MiddlewareQueue

	closeChan chan struct{}
}

func New(conf string) *Server {
	mux := http.NewServeMux()

	if err := env.Init(conf); err != nil {
		fmt.Fprintf(os.Stderr, "env init error: %v", err)
		return nil
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
	mq.Use(LoggerAsMiddleware(logInst, panicLogger), TrackerMiddleware(), ContextAsMiddleware(), TimeoutMiddleware())

	if env.EnablePprof() {
		mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
		mux.HandleFunc("/debug/pprof/cmdline", http.DefaultServeMux.ServeHTTP)
		mux.HandleFunc("/debug/pprof/profile", http.DefaultServeMux.ServeHTTP)
		mux.HandleFunc("/debug/pprof/symbol", http.DefaultServeMux.ServeHTTP)
		mux.HandleFunc("/debug/pprof/trace", http.DefaultServeMux.ServeHTTP)
	}

	return &Server{
		network:     env.Network(),
		addr:        env.Addr(),
		mux:         mux,
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
		r = r.WithContext(ctx)
		gcx := GetContext(ctx)
		gcx.SetContextOptions(WithRequest(r), WithResponseWriter(w))

		cloned := CloneController(controller)
		controllerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			err := cloned.Init(ctx)
			if err != nil {
				return
			}
			err = cloned.SanityCheck(ctx)
			if err != nil {
				return
			}
			err = cloned.ParseRequest(ctx, gcx.RawBody)
			if err != nil {
				return
			}

			err = cloned.Serve(ctx)
			if err != nil {
				return
			}
			err = cloned.Finalize(ctx)
			if err != nil {
				return
			}
		})

		wrappedHandler := s.mq.Apply(controllerHandler)
		wrappedHandler.ServeHTTP(w, r)
	}

	s.mux.HandleFunc(path, handler)
}

func (s *Server) ServeFile(path, realPath string) {
	if !filepath.IsAbs(realPath) {
		realPath = filepath.Join(env.RootDir(), realPath)
	}
	realPath = filepath.Clean(realPath)

	_, err := os.Stat(realPath)
	if err != nil {
		panic(fmt.Sprintf("path err %v", err))
	}

	fileServer := http.FileServer(http.Dir(realPath))
	s.mux.Handle(path+"/", http.StripPrefix(path, fileServer))
}

func (s *Server) UseMiddleware(middlewares ...HandlerMiddleware) {
	s.mq.Use(middlewares...)
}
