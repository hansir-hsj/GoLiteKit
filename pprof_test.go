package golitekit

import (
	"net/http"
	"net/http/httptest"
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
	app.Router.MountPprof()

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
	app.Router.MountPprof(PprofOptions{LoopbackOnly: true})

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
	app.Router.MountPprof(PprofOptions{LoopbackOnly: true})

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for loopback", rec.Code, http.StatusOK)
	}
}
