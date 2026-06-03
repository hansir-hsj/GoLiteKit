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
	Services *Services
	Router   *Router

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

	router.Use(
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
		LoggerAsMiddleware(services.logger, services.panicLogger),
		TrackerMiddleware(),
		TimeoutMiddleware(),
		ContextAsMiddleware(),
	)

	return &App{
		Services: services,
		Router:   router,
	}
}

// NewAppFromConfig creates an App from config file (for backward compatibility).
func NewAppFromConfig(confPath string, opts ...ServiceOption) (*App, error) {
	if err := env.Init(confPath); err != nil {
		return nil, err
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

	router.Use(
		ErrorHandlerMiddleware(
			WithErrorCallback(func(r *http.Request, err *AppError) {
				services.logger.Warning(r.Context(), "request error: %d %s", err.Code, err.Message)
			}),
			WithPanicCallback(func(r *http.Request, recovered any) {
				services.panicLogger.Report(r.Context(), recovered)
			}),
		),
		LoggerAsMiddleware(services.logger, services.panicLogger),
		TrackerMiddleware(),
		TimeoutMiddleware(),
		ContextAsMiddleware(),
	)

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
		Services: services,
		Router:   router,
	}, nil
}

func appServerConfig(configs []ServerConfig) ServerConfig {
	if len(configs) > 0 {
		return configs[0]
	}
	return DefaultServerConfig()
}

func (a *App) setServer(srv *Server) {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	a.server = srv
}

func (a *App) currentServer() *Server {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	return a.server
}

// Route registration shortcuts — delegate to the embedded Router.
func (a *App) GET(path string, c any)           { a.Router.GET(path, c) }
func (a *App) POST(path string, c any)          { a.Router.POST(path, c) }
func (a *App) PUT(path string, c any)           { a.Router.PUT(path, c) }
func (a *App) DELETE(path string, c any)        { a.Router.DELETE(path, c) }
func (a *App) PATCH(path string, c any)         { a.Router.PATCH(path, c) }
func (a *App) HEAD(path string, c any)          { a.Router.HEAD(path, c) }
func (a *App) OPTIONS(path string, c any)       { a.Router.OPTIONS(path, c) }
func (a *App) Any(path string, c any)           { a.Router.Any(path, c) }
func (a *App) Use(middlewares ...Middleware)    { a.Router.Use(middlewares...) }
func (a *App) Group(prefix string) *RouterGroup { return a.Router.Group(prefix) }
func (a *App) Static(urlPath, fsPath string)    { a.Router.Static(urlPath, fsPath) }
func (a *App) Handler() http.Handler            { return a.Router.Handler() }

// MountPprof registers the standard net/http/pprof endpoints on the app router.
// It only mounts handlers and does not start or block the server; pass PprofOptions
// to restrict access or change the mount prefix.
func (a *App) MountPprof(opts ...PprofOptions) { a.Router.MountPprof(opts...) }

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
	if err := srv.Start(a.Router.Handler()); err != nil {
		return err
	}
	a.server = srv
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
	if err := srv.Start(a.Router.Handler()); err != nil {
		a.serverMu.Unlock()
		return err
	}
	a.server = srv
	a.serverMu.Unlock()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), srv.config.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
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

// Run starts the server on the given address.
func (a *App) Run(addr string) error {
	server := NewServer(ServerConfig{Addr: addr})
	return server.Run(a.Router.Handler())
}

// RunWithConfig starts the server with custom config.
func (a *App) RunWithConfig(config ServerConfig) error {
	server := NewServer(config)
	return server.Run(a.Router.Handler())
}

// RunFromEnv starts the server using env config.
func (a *App) RunFromEnv() error {
	config := ServerConfig{
		Addr:              env.Addr(),
		Network:           env.Network(),
		ReadTimeout:       env.ReadTimeout(),
		WriteTimeout:      env.WriteTimeout(),
		IdleTimeout:       env.IdleTimeout(),
		ReadHeaderTimeout: env.ReadHeaderTimeout(),
		MaxHeaderBytes:    env.MaxHeaderBytes(),
		ShutdownTimeout:   env.ShutdownTimeout(),
	}
	if env.TLS() {
		config.TLSCertFile = env.TLSCertFile()
		config.TLSKeyFile = env.TLSKeyFile()
	}
	return a.RunWithConfig(config)
}
