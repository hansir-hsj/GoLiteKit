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

	mux            *http.ServeMux
	methodHandlers map[string]map[string]http.Handler // path -> method -> handler
	listener       net.Listener
	httpServer     http.Server

	logger      logger.Logger
	panicLogger *logger.PanicLogger

	mq MiddlewareQueue

	closeChan chan struct{}
}

// New creates a new Server instance with the given configuration file.
// Returns an error if the configuration, logger, or panic logger initialization fails.
func New(conf string) (*Server, error) {
	mux := http.NewServeMux()

	if err := env.Init(conf); err != nil {
		return nil, fmt.Errorf("env init error: %w", err)
	}

	logInst, err := logger.NewLogger(env.LoggerConfigFile())
	if err != nil {
		return nil, fmt.Errorf("logger init error: %w", err)
	}
	panicLogger, err := logger.NewPanicLogger(env.LoggerConfigFile())
	if err != nil {
		return nil, fmt.Errorf("panic logger init error: %w", err)
	}

	// inner middleware
	mq := NewMiddlewareQueue()
	// Execute sequence: ErrorHandler → Logger → Tracker → Timeout → Context → Controller
	mq.Use(
		ErrorHandlerMiddleware(
			WithErrorCallback(func(r *http.Request, err *AppError) {
				logInst.Warning(r.Context(), "request error: %d %s", err.Code, err.Message)
			}),
			WithPanicCallback(func(r *http.Request, recovered any) {
			}),
		),
		LoggerAsMiddleware(logInst, panicLogger),
		TrackerMiddleware(),
		TimeoutMiddleware(),
		ContextAsMiddleware(),
	)

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
	}, nil
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

	if staticDir := env.StaticDir(); staticDir != "" {
		s.ServeFile("/static", staticDir)
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
	case syscall.SIGINT, syscall.SIGTERM:
		fmt.Fprintf(os.Stderr, "%s receive signal %v, shutting down...\n", time.Now(), sig)
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
	s.registerHandlerWithMiddlewares(method, path, controller, nil)
}

func (s *Server) registerHandlerWithMiddlewares(method, path string, controller Controller, groupMiddlewares MiddlewareQueue) {
	// 初始化 methodHandlers map
	if s.methodHandlers == nil {
		s.methodHandlers = make(map[string]map[string]http.Handler)
	}

	// 创建当前 method 的 handler
	handler := s.createControllerHandler(controller, groupMiddlewares)

	// 检查该 path 是否已注册
	if s.methodHandlers[path] == nil {
		s.methodHandlers[path] = make(map[string]http.Handler)

		// 首次注册：向 mux 注册统一分发器
		s.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if h, ok := s.methodHandlers[path][r.Method]; ok {
				h.ServeHTTP(w, r)
			} else {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	// 将 handler 存入对应 method
	s.methodHandlers[path][method] = handler
}

// createControllerHandler 封装 controller 处理逻辑，返回 http.Handler
// groupMiddlewares 是路由组的中间件，会在全局中间件之后、Controller之前执行
func (s *Server) createControllerHandler(controller Controller, groupMiddlewares MiddlewareQueue) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				SetError(ctx, ErrInternal("Controller init failed", err))
				return
			}
			err = cloned.SanityCheck(ctx)
			if err != nil {
				SetError(ctx, ErrBadRequest("Sanity check failed", err))
				return
			}
			err = cloned.ParseRequest(ctx, gcx.RawBody)
			if err != nil {
				SetError(ctx, ErrBadRequest("Parse request failed", err))
				return
			}

			err = cloned.Serve(ctx)
			if err != nil {
				SetError(ctx, ErrInternal("Controller serve failed", err))
				return
			}
			err = cloned.Finalize(ctx)
			if err != nil {
				SetError(ctx, ErrInternal("Controller finalize failed", err))
				return
			}
		})

		// Apply group middlewares first (inner layer)
		var wrappedHandler http.Handler = controllerHandler
		if len(groupMiddlewares) > 0 {
			wrappedHandler = groupMiddlewares.Apply(wrappedHandler)
		}

		// Apply global middlewares (outer layer)
		wrappedHandler = s.mq.Apply(wrappedHandler)
		wrappedHandler.ServeHTTP(w, r)
	})
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
