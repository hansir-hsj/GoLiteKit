package golitekit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// App is the main entry point, combining Services and Router.
type App struct {
	services *Services
	router   *Router
	env      *env.Env // nil for NewApp

	deferredMounts []func()
	mountOnce      sync.Once

	closeMu   sync.Mutex
	closers   []func(context.Context) error
	closeOnce sync.Once
	closeErr  error

	pprofAddr    string // from the pprofAddr config; "" = not used
	pprofMu      sync.Mutex
	pprofStarted bool

	serverMu sync.Mutex
	server   *Server
}

// startPprof starts the separate pprof listener configured with pprofAddr,
// once. It is stopped by Close.
func (a *App) startPprof() error {
	a.pprofMu.Lock()
	defer a.pprofMu.Unlock()
	if a.pprofAddr == "" || a.pprofStarted {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := ServePprof(ctx, a.pprofAddr, PprofOptions{LoopbackOnly: true}); err != nil {
		cancel()
		return err
	}
	a.pprofStarted = true
	a.OnClose(func(context.Context) error { cancel(); return nil })
	return nil
}

// OnClose registers fn to run on Close, in reverse registration order.
func (a *App) OnClose(fn func(ctx context.Context) error) {
	if fn == nil {
		return
	}
	a.closeMu.Lock()
	a.closers = append(a.closers, fn)
	a.closeMu.Unlock()
}

// Close runs the OnClose functions in reverse order and closes the loggers
// created by NewAppFromConfig. Clients passed in by the caller are not closed.
// Only the first call has effect; errors are joined.
func (a *App) Close(ctx context.Context) error {
	a.closeOnce.Do(func() {
		a.closeMu.Lock()
		closers := a.closers
		a.closers = nil
		a.closeMu.Unlock()

		var errs []error
		for i := len(closers) - 1; i >= 0; i-- {
			if err := closers[i](ctx); err != nil {
				errs = append(errs, err)
			}
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
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

// NewAppFromConfig creates an App from a config file. The config is kept on
// the app (see App.Env) and also becomes the env package default.
func NewAppFromConfig(confPath string, opts ...ServiceOption) (*App, error) {
	e, err := env.Load(confPath)
	if err != nil {
		return nil, err
	}
	env.SetDefault(e)

	loggerOptions := LoggerOptions{
		LogRequestBody:  e.LogRequestBody(),
		LogResponseBody: e.LogResponseBody(),
		MaxBodyBytes:    DefaultLogBodyLimit,
	}
	timeoutOptions := TimeoutOptions{
		Duration:   e.WriteTimeout(),
		SSETimeout: e.SSETimeout(),
	}

	services := &Services{}
	for _, opt := range opts {
		opt(services)
	}

	// Loggers created here are closed by the app; passed-in ones are not.
	var ownedLoggers []func() error
	if services.logger == nil {
		// An empty path falls back to the console logger.
		l, err := logger.NewLogger(e.LoggerConfigFile())
		if err != nil {
			return nil, err
		}
		services.logger = l
		ownedLoggers = append(ownedLoggers, l.Close)
	}
	if services.panicLogger == nil {
		pl, err := logger.NewPanicLogger(e.LoggerConfigFile())
		if err != nil {
			for _, closeLogger := range ownedLoggers {
				_ = closeLogger()
			}
			return nil, err
		}
		services.panicLogger = pl
		ownedLoggers = append(ownedLoggers, pl.Close)
	}

	router := NewRouter(services)
	router.Use(defaultMiddlewares(services, defaultMiddlewareOptions{
		logger:  loggerOptions,
		timeout: timeoutOptions,
	})...)

	app := &App{
		services: services,
		router:   router,
		env:      e,
	}
	// Registered first so they are closed last: other close functions may log.
	for _, closeLogger := range ownedLoggers {
		closeLogger := closeLogger
		app.OnClose(func(context.Context) error { return closeLogger() })
	}

	// Mounted on first serve, so Use still works after NewAppFromConfig.
	if e.EnablePprof() {
		if addr := e.PprofAddr(); addr != "" {
			app.pprofAddr = addr // separate listener, started with the server
		} else {
			app.deferredMounts = append(app.deferredMounts, func() {
				router.MountPprof(PprofOptions{LoopbackOnly: true})
			})
		}
	}
	// StaticDir is already absolute (resolved against the working directory).
	if staticDir := e.StaticDir(); staticDir != "" {
		if _, err := os.Stat(staticDir); err == nil {
			app.deferredMounts = append(app.deferredMounts, func() {
				router.Static("/static", staticDir)
			})
		}
	}

	return app, nil
}

// mountDeferred registers config-driven routes once, before serving.
func (a *App) mountDeferred() {
	a.mountOnce.Do(func() {
		for _, mount := range a.deferredMounts {
			mount()
		}
	})
}

type defaultMiddlewareOptions struct {
	logger  LoggerOptions
	timeout TimeoutOptions
}

// defaultMiddlewares returns the default stack, outermost first. Logger and
// LogID sit outside the error handler so they also cover error responses.
func defaultMiddlewares(services *Services, opts defaultMiddlewareOptions) []Middleware {
	middlewares := []Middleware{}
	if observabilityMiddleware := services.ObservabilityMiddleware(); observabilityMiddleware != nil {
		middlewares = append(middlewares, observabilityMiddleware)
	}
	middlewares = append(middlewares,
		LoggerAsMiddleware(services.logger, services.panicLogger, opts.logger),
		LogIDMiddleware(services.logIDOptions),
		ErrorHandlerMiddleware(),
		TimeoutMiddleware(opts.timeout),
		ContextAsMiddleware(),
	)
	return middlewares
}

// serverConfig picks the explicit config, else the loaded config file, else
// DefaultServerConfig.
func (a *App) serverConfig(configs []ServerConfig) ServerConfig {
	if len(configs) > 0 || a.env == nil {
		return appServerConfig(configs)
	}
	e := a.env
	config := ServerConfig{
		Addr:              e.Addr(),
		Network:           e.Network(),
		ReadTimeout:       e.ReadTimeout(),
		WriteTimeout:      e.WriteTimeout(),
		IdleTimeout:       e.IdleTimeout(),
		ReadHeaderTimeout: e.ReadHeaderTimeout(),
		MaxHeaderBytes:    e.MaxHeaderBytes(),
		ShutdownTimeout:   e.ShutdownTimeout(),
	}
	if e.TLS() {
		config.TLSCertFile = e.TLSCertFile()
		config.TLSKeyFile = e.TLSKeyFile()
	}
	return config
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

// Env returns the config loaded by NewAppFromConfig. It is nil for apps built
// with NewApp; its accessors are nil-safe and then return the defaults.
func (a *App) Env() *env.Env { return a.env }

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

// UseOuter registers middlewares that run before the default middleware stack;
// see Router.UseOuter.
func (a *App) UseOuter(middlewares ...Middleware) { a.router.UseOuter(middlewares...) }

// Handler returns the app's http.Handler. The first call also mounts routes
// enabled by the config file (pprof, static), after which Use panics.
func (a *App) Handler() http.Handler {
	a.mountDeferred()
	return a.router.Handler()
}

// MountPprof registers the standard net/http/pprof endpoints on the app router.
// It only mounts handlers and does not start or block the server; pass PprofOptions
// to restrict access or change the mount prefix.
func (a *App) MountPprof(opts ...PprofOptions) { a.router.MountPprof(opts...) }

// Start starts the server in the background. Without a config, an app from
// NewAppFromConfig uses its config file, others DefaultServerConfig.
func (a *App) Start(configs ...ServerConfig) error {
	if err := a.startPprof(); err != nil {
		return err
	}
	a.serverMu.Lock()
	defer a.serverMu.Unlock()

	if a.server != nil {
		return fmt.Errorf("app server already started")
	}

	srv := NewServer(a.serverConfig(configs))
	if err := srv.Start(a.Handler()); err != nil {
		return err
	}
	a.server = srv
	go a.clearServerWhenDone(srv)
	return nil
}

// ListenAndServe serves until ctx is canceled, then shuts down gracefully and
// closes the app (see Close). The config is chosen as in Start.
func (a *App) ListenAndServe(ctx context.Context, configs ...ServerConfig) error {
	if err := a.startPprof(); err != nil {
		return err
	}
	a.serverMu.Lock()
	if a.server != nil {
		a.serverMu.Unlock()
		return fmt.Errorf("app server already started")
	}

	config := a.serverConfig(configs)
	srv := NewServer(config)
	if err := srv.Start(a.Handler()); err != nil {
		a.serverMu.Unlock()
		return err
	}
	a.server = srv
	a.serverMu.Unlock()

	select {
	case serveErr := <-srv.Done():
		a.clearServer(srv)
		closeCtx, cancel := context.WithTimeout(context.Background(), srv.config.ShutdownTimeout)
		defer cancel()
		return errors.Join(serveErr, a.Close(closeCtx))
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), srv.config.ShutdownTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		// Shutdown timed out: drop the remaining connections.
		_ = srv.Close()
	}
	a.clearServer(srv)
	// shutdownCtx may have expired.
	closeCtx, cancelClose := context.WithTimeout(context.Background(), srv.config.ShutdownTimeout)
	defer cancelClose()
	return errors.Join(shutdownErr, a.Close(closeCtx))
}

// Shutdown gracefully stops the current server. It does not close the app,
// so the app can be started again.
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
	a.clearServer(srv)
}

func (a *App) clearServer(srv *Server) {
	a.serverMu.Lock()
	if a.server == srv {
		a.server = nil
	}
	a.serverMu.Unlock()
}
