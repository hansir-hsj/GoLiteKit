package golitekit

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPprof_NotMountedByDefault(t *testing.T) {
	app := NewApp()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("pprof should not be mounted by default")
	}
}

func TestPprof_MountedExplicitly(t *testing.T) {
	app := NewApp()
	app.MountPprof()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPprof_UsesCurrentMiddlewareAndFreezesUse(t *testing.T) {
	executed := false
	app := NewApp()
	app.Use(func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			executed = true
			return next(ctx, w, r)
		}
	})
	app.MountPprof()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !executed {
		t.Fatal("expected pprof route to run current middleware")
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		msg := recovered.(string)
		if !strings.Contains(msg, "middleware must be registered before routes") {
			t.Fatalf("panic = %q, want middleware ordering guidance", msg)
		}
	}()

	app.Use(func(next Handler) Handler {
		return next
	})
}

func TestApp_MountPprof(t *testing.T) {
	app := NewApp()
	app.MountPprof(PprofOptions{LoopbackOnly: true})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestPprof_LoopbackOnly_BlocksRemote(t *testing.T) {
	app := NewApp()
	app.MountPprof(PprofOptions{LoopbackOnly: true})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d for non-loopback", rec.Code, http.StatusForbidden)
	}
}

func TestPprof_LoopbackOnly_AllowsLocal(t *testing.T) {
	app := NewApp()
	app.MountPprof(PprofOptions{LoopbackOnly: true})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for loopback", rec.Code, http.StatusOK)
	}
}

// Behind a reverse proxy on the same host, external requests arrive from
// 127.0.0.1; forwarding headers reveal them.
func TestPprof_LoopbackOnly_BlocksProxiedRequests(t *testing.T) {
	app := NewApp()
	app.MountPprof(PprofOptions{LoopbackOnly: true})

	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded"} {
		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set(h, "203.0.113.9")
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", h, rec.Code)
		}
	}
}

func TestServePprofOnSeparateListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := ServePprof(ctx, "127.0.0.1:0", PprofOptions{LoopbackOnly: true})
	if err != nil {
		t.Fatalf("ServePprof: %v", err)
	}

	resp, err := http.Get("http://" + addr.String() + "/debug/pprof/cmdline")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNewAppFromConfigPprofAddrKeepsMainPortClean(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pprofAddr := ln.Addr().String()
	ln.Close()

	conf := filepath.Join(t.TempDir(), "app.toml")
	content := "[HttpServer]\nenablePprof = true\npprofAddr = \"" + pprofAddr + "\"\n"
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	app, err := NewAppFromConfig(conf)
	if err != nil {
		t.Fatalf("NewAppFromConfig: %v", err)
	}
	if err := app.Start(ServerConfig{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		app.Shutdown(context.Background())
		app.Close(context.Background())
	}()

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("main port pprof status = %d, want 404", rec.Code)
	}

	resp, err := http.Get("http://" + pprofAddr + "/debug/pprof/cmdline")
	if err != nil {
		t.Fatalf("GET pprof listener: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pprof listener status = %d, want 200", resp.StatusCode)
	}
}
