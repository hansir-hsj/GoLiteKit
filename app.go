package golitekit

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// App is the main entry point, combining Services and Router.
type App struct {
	Services *Services
	Router   *Router
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
		router.mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
		router.mux.HandleFunc("/debug/pprof/cmdline", http.DefaultServeMux.ServeHTTP)
		router.mux.HandleFunc("/debug/pprof/profile", http.DefaultServeMux.ServeHTTP)
		router.mux.HandleFunc("/debug/pprof/symbol", http.DefaultServeMux.ServeHTTP)
		router.mux.HandleFunc("/debug/pprof/trace", http.DefaultServeMux.ServeHTTP)
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

// Route registration shortcuts — delegate to the embedded Router.
func (a *App) GET(path string, c Controller)        { a.Router.GET(path, c) }
func (a *App) POST(path string, c Controller)       { a.Router.POST(path, c) }
func (a *App) PUT(path string, c Controller)        { a.Router.PUT(path, c) }
func (a *App) DELETE(path string, c Controller)     { a.Router.DELETE(path, c) }
func (a *App) PATCH(path string, c Controller)      { a.Router.PATCH(path, c) }
func (a *App) HEAD(path string, c Controller)       { a.Router.HEAD(path, c) }
func (a *App) OPTIONS(path string, c Controller)    { a.Router.OPTIONS(path, c) }
func (a *App) Any(path string, c Controller)        { a.Router.Any(path, c) }
func (a *App) Use(middlewares ...HandlerMiddleware) { a.Router.Use(middlewares...) }
func (a *App) Group(prefix string) *RouterGroup     { return a.Router.Group(prefix) }
func (a *App) Static(urlPath, fsPath string)        { a.Router.Static(urlPath, fsPath) }
func (a *App) Handler() http.Handler                { return a.Router.Handler() }

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
