package golitekit

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

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
