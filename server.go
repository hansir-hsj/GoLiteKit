package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

type Server struct {
	addr        string
	router      *Router
	rateLimiter *RateLimiter
	mq          MiddlewareQueue

	httpServer http.Server
	closeChan  chan struct{}

	logger      logger.Logger
	panicLogger *logger.PanicLogger
}

func New(conf string) *Server {
	router := NewRouter()

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

	mq := NewMiddlewareQueue(LoggerAsMiddleware(logInst, panicLogger), TrackerMiddleware, ContextAsMiddleware(), TimeoutMiddleware)

	return &Server{
		addr:        env.Addr(),
		router:      router,
		rateLimiter: rateLimiter,
		closeChan:   make(chan struct{}),
		mq:          mq,
		logger:      logInst,
		panicLogger: panicLogger,
	}
}

func (s *Server) Start() {
	s.httpServer = http.Server{
		Addr:           s.addr,
		ReadTimeout:    env.ReadTimeout(),
		WriteTimeout:   env.WriteTimeout(),
		IdleTimeout:    env.IdleTimeout(),
		MaxHeaderBytes: env.MaxHeaderBytes(),
		Handler:        s,
	}

	if env.ReadHeaderTimeout() > 0 {
		s.httpServer.ReadHeaderTimeout = env.ReadHeaderTimeout()
	}

	go s.handleSignal()

	var err error
	certFile := env.TLSCertFile()
	keyFile := env.TLSKeyFile()
	if certFile != "" && keyFile != "" {
		s.httpServer.ListenAndServeTLS(certFile, keyFile)
	} else {
		err = s.httpServer.ListenAndServe()
	}

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
	ctx, cancel := context.WithTimeout(context.Background(), env.ShutdownTimeout())
	defer cancel()

	s.httpServer.Shutdown(ctx)
	s.closeChan <- struct{}{}
}

func (s *Server) NewRouterGroup(prefix string) *RouterGroup {
	group := newRouterGroup(prefix)
	group.server = s
	return group
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

func (s *Server) Static(path, realPath string) {
	if !filepath.IsAbs(realPath) {
		realPath = filepath.Join(env.RootDir(), realPath)
	}
	realPath = filepath.Clean(realPath)

	_, err := os.Stat(realPath)
	if err != nil {
		panic(fmt.Sprintf("path err %v", err))
	}

	filepath.Walk(realPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(realPath, p)
		if err != nil {
			return err
		}
		tmpPath := filepath.Join(path, relPath)
		if info.IsDir() {
			if !strings.HasSuffix(p, "/") {
				p = p + "/"
			}
		}

		s.router.Static(tmpPath, &StaticController{
			Path: p,
		})

		return nil
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := WithContext(req.Context())
	ctx = logger.WithLoggerContext(ctx)
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithRequest(req), WithResponseWriter(w))

	mq := s.mq.Clone()

	if s.rateLimiter != nil {
		mq.Use(s.rateLimiter.RateLimiterAsMiddleware())
	}

	controller, params, ok := s.router.Route(req.Method, req.URL.Path)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if params != nil {
		gcx.SetContextOptions(WithRouterParams(params))
	}

	cloned := CloneController(controller)

	mq.Use(controllerAsMiddleware(cloned))

	mq.Next(ctx)
}
