package golitekit

import (
	"context"
	"net/http"
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
