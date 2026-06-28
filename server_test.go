package golitekit

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultServerConfig_HasSafeTimeouts(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.ReadTimeout <= 0 {
		t.Fatal("ReadTimeout should be positive")
	}
	if cfg.ReadHeaderTimeout <= 0 {
		t.Fatal("ReadHeaderTimeout should be positive")
	}
	if cfg.WriteTimeout <= 0 {
		t.Fatal("WriteTimeout should be positive")
	}
	if cfg.IdleTimeout <= 0 {
		t.Fatal("IdleTimeout should be positive")
	}
	if cfg.MaxHeaderBytes <= 0 {
		t.Fatal("MaxHeaderBytes should be positive")
	}
}

func TestNewServer_FillsSafeDefaults(t *testing.T) {
	defaults := DefaultServerConfig()
	srv := NewServer(ServerConfig{})

	if srv.config.Addr != defaults.Addr {
		t.Fatalf("Addr = %q, want %q", srv.config.Addr, defaults.Addr)
	}
	if srv.config.Network != defaults.Network {
		t.Fatalf("Network = %q, want %q", srv.config.Network, defaults.Network)
	}
	if srv.config.ReadTimeout != defaults.ReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", srv.config.ReadTimeout, defaults.ReadTimeout)
	}
	if srv.config.ReadHeaderTimeout != defaults.ReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", srv.config.ReadHeaderTimeout, defaults.ReadHeaderTimeout)
	}
	if srv.config.WriteTimeout != defaults.WriteTimeout {
		t.Fatalf("WriteTimeout = %v, want %v", srv.config.WriteTimeout, defaults.WriteTimeout)
	}
	if srv.config.IdleTimeout != defaults.IdleTimeout {
		t.Fatalf("IdleTimeout = %v, want %v", srv.config.IdleTimeout, defaults.IdleTimeout)
	}
	if srv.config.MaxHeaderBytes != defaults.MaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.config.MaxHeaderBytes, defaults.MaxHeaderBytes)
	}
	if srv.config.ShutdownTimeout != defaults.ShutdownTimeout {
		t.Fatalf("ShutdownTimeout = %v, want %v", srv.config.ShutdownTimeout, defaults.ShutdownTimeout)
	}
}

func TestNewServer_PreservesExplicitConfig(t *testing.T) {
	cfg := ServerConfig{
		Addr:              "127.0.0.1:0",
		Network:           "tcp4",
		ReadTimeout:       time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
		MaxHeaderBytes:    2048,
		ShutdownTimeout:   5 * time.Second,
	}

	srv := NewServer(cfg)

	if srv.config != cfg {
		t.Fatalf("config = %#v, want %#v", srv.config, cfg)
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if err := srv.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify server is listening
	addr := srv.Addr()
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestServer_StartRejectsRepeatedStart(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if err := srv.Start(handler); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Start(handler); err == nil {
		t.Fatal("expected second Start to fail")
	} else if !strings.Contains(err.Error(), "server already started") {
		t.Fatalf("second Start error = %v, want already-started", err)
	}
}

func TestServer_ConcurrentStart_AllowsOneServer(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- srv.Start(handler)
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	alreadyStarted := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if strings.Contains(err.Error(), "server already started") {
			alreadyStarted++
		}
	}

	if successes != 1 {
		t.Fatalf("successful starts = %d, want 1", successes)
	}
	if alreadyStarted != goroutines-1 {
		t.Fatalf("already-started errors = %d, want %d", alreadyStarted, goroutines-1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestServer_StartExposesServeErrors(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if err := srv.Start(handler); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ln := srv.currentListener()
	if ln == nil {
		t.Fatal("Start did not expose listener")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	select {
	case err := <-srv.Done():
		if err == nil {
			t.Fatal("expected serve error after listener close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not expose serve error")
	}
}

func TestServer_DoneCanBeReadWhileStarting(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	doneReady := make(chan struct{})
	startErr := make(chan error, 1)
	go func() {
		close(doneReady)
		startErr <- srv.Start(handler)
	}()

	<-doneReady
	_ = srv.Done()

	if err := <-startErr; err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestServer_ListenAndServe_ContextCancel(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe(ctx, handler)
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	// Cancel context triggers shutdown
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ListenAndServe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancel")
	}
}

func TestServer_ListenAndServe_ReturnsServeError(t *testing.T) {
	srv := NewServer(ServerConfig{Addr: "127.0.0.1:0"})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe(ctx, handler)
	}()

	var ln net.Listener
	for i := 0; i < 50; i++ {
		ln = srv.currentListener()
		if ln != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ln == nil {
		t.Fatal("ListenAndServe did not start listener")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected serve error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return after serve error")
	}
}

func waitForAppServer(t *testing.T, app *App) *Server {
	t.Helper()

	for i := 0; i < 50; i++ {
		srv := app.currentServer()
		if srv != nil {
			return srv
		}
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}

func TestApp_StartAndShutdown(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("App.Start: %v", err)
	}
	srv := app.currentServer()
	if srv == nil {
		t.Fatal("App.Start did not store server")
	}

	resp, err := http.Get("http://" + srv.Addr() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("App.Shutdown: %v", err)
	}
}

func TestApp_ListenAndServe_ContextCancel(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.ListenAndServe(ctx, ServerConfig{Addr: "127.0.0.1:0"})
	}()

	srv := waitForAppServer(t, app)
	if srv == nil {
		cancel()
		t.Fatal("App.ListenAndServe did not start listener")
	}
	if srv.Addr() == "" {
		cancel()
		t.Fatal("App.ListenAndServe did not expose listener address")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("App.ListenAndServe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("App.ListenAndServe did not return after context cancel")
	}
}

func TestApp_ListenAndServe_ReturnsServeError(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.ListenAndServe(ctx, ServerConfig{Addr: "127.0.0.1:0"})
	}()

	srv := waitForAppServer(t, app)
	if srv == nil {
		t.Fatal("App.ListenAndServe did not start listener")
	}
	ln := srv.currentListener()
	if ln == nil {
		t.Fatal("App.ListenAndServe did not expose listener")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected serve error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("App.ListenAndServe did not return after serve error")
	}

	if srv := app.currentServer(); srv != nil {
		t.Fatalf("App.ListenAndServe left current server set after serve error: %p", srv)
	}
}

func TestApp_StartClearsServerAfterServeError(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("App.Start: %v", err)
	}
	srv := app.currentServer()
	if srv == nil {
		t.Fatal("App.Start did not store server")
	}
	ln := srv.currentListener()
	if ln == nil {
		t.Fatal("App.Start did not expose listener")
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	for i := 0; i < 50; i++ {
		if app.currentServer() == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if srv := app.currentServer(); srv != nil {
		t.Fatalf("App.Start left current server set after serve error: %p", srv)
	}

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("App.Start after serve error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("App.Shutdown: %v", err)
	}
}

func TestApp_StartAfterShutdown(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("first App.Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("first App.Shutdown: %v", err)
	}

	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("second App.Start after shutdown: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := app.Shutdown(ctx2); err != nil {
		t.Fatalf("second App.Shutdown: %v", err)
	}
}

func TestApp_ListenAndServe_ClearsServerAfterCancel(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- app.ListenAndServe(ctx, ServerConfig{Addr: "127.0.0.1:0"})
	}()

	if srv := waitForAppServer(t, app); srv == nil {
		cancel()
		t.Fatal("App.ListenAndServe did not start listener")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("App.ListenAndServe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("App.ListenAndServe did not return after context cancel")
	}

	if srv := app.currentServer(); srv != nil {
		t.Fatalf("App.ListenAndServe left current server set after cancel: %p", srv)
	}
}

func TestApp_ConcurrentStart_AllowsOneServer(t *testing.T) {
	app := NewApp()
	app.GET("/", func(ctx *Context) error {
		return ctx.String(http.StatusOK, "ok")
	})

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- app.Start(ServerConfig{Addr: "127.0.0.1:0"})
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	alreadyStarted := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if strings.Contains(err.Error(), "app server already started") {
			alreadyStarted++
		}
	}

	if successes != 1 {
		t.Fatalf("successful starts = %d, want 1", successes)
	}
	if alreadyStarted != goroutines-1 {
		t.Fatalf("already-started errors = %d, want %d", alreadyStarted, goroutines-1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("App.Shutdown: %v", err)
	}
}
