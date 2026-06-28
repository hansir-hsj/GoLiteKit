package golitekit

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// App is the main entry point, combining Services and Router.
type App struct {
	services *Services
	router   *Router

	serverMu sync.Mutex
	server   *Server
}

// NewApp creates an App with dependency injection.
func NewApp(opts ...ServiceOption) *App {
	services := &Services{}
	for _, opt := range opts {
		opt(services)
	}

	router := NewRouter(services)
	router.Use(defaultMiddlewares(services, defaultMiddlewareOptions{})...)

	return &App{
		services: services,
		router:   router,
	}
}

// NewAppFromConfig creates an App from an env config file.
func NewAppFromConfig(confPath string, opts ...ServiceOption) (*App, error) {
	if err := env.Init(confPath); err != nil {
		return nil, err
	}
	loggerOptions := LoggerOptions{
		LogRequestBody:  env.LogRequestBody(),
		LogResponseBody: env.LogResponseBody(),
		MaxBodyBytes:    DefaultLogBodyLimit,
	}
	timeoutOptions := TimeoutOptions{
		Duration:   env.WriteTimeout(),
		SSETimeout: env.SSETimeout(),
	}

	services := &Services{}
	for _, opt := range opts {
		opt(services)
	}

	if services.logger == nil {
		loggerCfg := env.LoggerConfigFile()
		var l logger.Logger
		var err error
		if loggerCfg == "" {
			l, err = logger.NewLogger() // no config; fall back to console logger
		} else {
			l, err = logger.NewLogger(loggerCfg)
		}
		if err != nil {
			return nil, err
		}
		services.logger = l
	}
	if services.panicLogger == nil {
		pl, err := logger.NewPanicLogger(env.LoggerConfigFile())
		if err != nil {
			return nil, err
		}
		services.panicLogger = pl
	}

	router := NewRouter(services)
	router.Use(defaultMiddlewares(services, defaultMiddlewareOptions{
		logger:  loggerOptions,
		timeout: timeoutOptions,
	})...)

	if env.EnablePprof() {
		router.MountPprof(PprofOptions{LoopbackOnly: true})
	}

	if staticDir := env.StaticDir(); staticDir != "" {
		if !filepath.IsAbs(staticDir) {
			staticDir = filepath.Join(env.RootDir(), staticDir)
		}
		if _, err := os.Stat(staticDir); err == nil {
			router.Static("/static", staticDir)
		}
	}

	return &App{
		services: services,
		router:   router,
	}, nil
}

type defaultMiddlewareOptions struct {
	logger  LoggerOptions
	timeout TimeoutOptions
}

func defaultMiddlewares(services *Services, opts defaultMiddlewareOptions) []Middleware {
	middlewares := []Middleware{}
	if observabilityMiddleware := services.ObservabilityMiddleware(); observabilityMiddleware != nil {
		middlewares = append(middlewares, observabilityMiddleware)
	}
	middlewares = append(middlewares,
		ErrorHandlerMiddleware(
			WithErrorCallback(func(r *http.Request, err *AppError) {
				if services.logger != nil {
					services.logger.Warning(r.Context(), "request error: %d %s", err.Code, err.Message)
				}
			}),
			WithPanicCallback(func(r *http.Request, recovered any) {
				if services.panicLogger != nil {
					services.panicLogger.Report(r.Context(), recovered)
				}
			}),
		),
		LoggerAsMiddleware(services.logger, services.panicLogger, opts.logger),
		LogIDMiddleware(),
		TimeoutMiddleware(opts.timeout),
		ContextAsMiddleware(),
	)
	return middlewares
}

func appServerConfig(configs []ServerConfig) ServerConfig {
	if len(configs) > 0 {
		return configs[0]
	}
	return DefaultServerConfig()
}

func (a *App) currentServer() *Server {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	return a.server
}

// Services returns the app dependency container.
func (a *App) Services() *Services { return a.services }

// Route registration shortcuts delegate to the app router.
func (a *App) GET(path string, c any)           { a.router.GET(path, c) }
func (a *App) POST(path string, c any)          { a.router.POST(path, c) }
func (a *App) PUT(path string, c any)           { a.router.PUT(path, c) }
func (a *App) DELETE(path string, c any)        { a.router.DELETE(path, c) }
func (a *App) PATCH(path string, c any)         { a.router.PATCH(path, c) }
func (a *App) HEAD(path string, c any)          { a.router.HEAD(path, c) }
func (a *App) OPTIONS(path string, c any)       { a.router.OPTIONS(path, c) }
func (a *App) Any(path string, c any)           { a.router.Any(path, c) }
func (a *App) Use(middlewares ...Middleware)    { a.router.Use(middlewares...) }
func (a *App) Group(prefix string) *RouterGroup { return a.router.Group(prefix) }
func (a *App) Static(urlPath, fsPath string)    { a.router.Static(urlPath, fsPath) }
func (a *App) Handler() http.Handler            { return a.router.Handler() }

// MountPprof registers the standard net/http/pprof endpoints on the app router.
// It only mounts handlers and does not start or block the server; pass PprofOptions
// to restrict access or change the mount prefix.
func (a *App) MountPprof(opts ...PprofOptions) { a.router.MountPprof(opts...) }

// Start starts the app's HTTP server in the background using the provided config,
// or DefaultServerConfig when no config is supplied. It returns after the listener
// is started and does not block while serving requests. If the app already has a
// running server, Start returns an already-started error.
func (a *App) Start(configs ...ServerConfig) error {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()

	if a.server != nil {
		return fmt.Errorf("app server already started")
	}

	srv := NewServer(appServerConfig(configs))
	if err := srv.Start(a.router.Handler()); err != nil {
		return err
	}
	a.server = srv
	go a.clearServerWhenDone(srv)
	return nil
}

// ListenAndServe starts the app's HTTP server and blocks until ctx is canceled.
// It uses the provided config, or DefaultServerConfig when no config is supplied.
// When ctx is canceled, ListenAndServe performs a graceful shutdown using the
// configured shutdown timeout, clears the current server, and returns nil for
// normal context-cancel shutdown. If the app already has a running server,
// ListenAndServe returns an already-started error.
func (a *App) ListenAndServe(ctx context.Context, configs ...ServerConfig) error {
	a.serverMu.Lock()
	if a.server != nil {
		a.serverMu.Unlock()
		return fmt.Errorf("app server already started")
	}

	config := appServerConfig(configs)
	srv := NewServer(config)
	if err := srv.Start(a.router.Handler()); err != nil {
		a.serverMu.Unlock()
		return err
	}
	a.server = srv
	a.serverMu.Unlock()

	select {
	case serveErr := <-srv.Done():
		a.serverMu.Lock()
		if a.server == srv {
			a.server = nil
		}
		a.serverMu.Unlock()
		return serveErr
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), srv.config.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}

	a.serverMu.Lock()
	if a.server == srv {
		a.server = nil
	}
	a.serverMu.Unlock()
	return nil
}

// Shutdown gracefully stops the app's current HTTP server using ctx and clears it
// after a successful shutdown. Shutdown blocks until the server stops or ctx is
// done. Calling Shutdown when no server is running is a no-op and returns nil.
func (a *App) Shutdown(ctx context.Context) error {
	a.serverMu.Lock()
	srv := a.server
	a.serverMu.Unlock()

	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}

	a.serverMu.Lock()
	if a.server == srv {
		a.server = nil
	}
	a.serverMu.Unlock()
	return nil
}

func (a *App) clearServerWhenDone(srv *Server) {
	<-srv.Done()

	a.serverMu.Lock()
	if a.server == srv {
		a.server = nil
	}
	a.serverMu.Unlock()
}
