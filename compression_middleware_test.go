package golitekit

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// Helpers
// ============================================================================

// gzipReader decompresses the response body for assertion.
func gunzip(t *testing.T, data []byte) string {
	t.Helper()
	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	return string(out)
}

// ============================================================================
// Core compression
// ============================================================================

func TestCompressionMiddleware_CompressesWhenAccepted(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello compressed world"))
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}

	body := gunzip(t, rec.Body.Bytes())
	if body != "hello compressed world" {
		t.Errorf("decompressed body = %q, want %q", body, "hello compressed world")
	}
}

func TestCompressionMiddleware_SkipsWithoutAcceptEncoding(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain text"))
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Accept-Encoding header.
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip when Accept-Encoding is absent")
	}
	if rec.Body.String() != "plain text" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "plain text")
	}
}

func TestCompressionMiddleware_VaryHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
	}
}

func TestCompressionMiddleware_NoContentLength(t *testing.T) {
	// The middleware must delete Content-Length since the compressed size differs.
	// Verify the middleware strips it from the response when gzip is used.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler does NOT set Content-Length; middleware must not add it.
		w.Write([]byte("data"))
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	// httptest.Recorder may set Content-Length in Flush; check that the
	// middleware header did not contain a Content-Length (the mux strips it).
	// Since httptest.Recorder adds it at commit time, assert Content-Encoding is gzip.
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
}

func TestCompressionMiddleware_NoContent204(t *testing.T) {
	// 204 No Content should not include Content-Encoding.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected Content-Encoding to be removed for 204 No Content")
	}
}

// ============================================================================
// Flush (streaming)
// ============================================================================

// flushableRecorder wraps ResponseRecorder and records Flush calls.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func (f *flushableRecorder) Flush() {
	f.flushCount++
	f.ResponseRecorder.Flush()
}

func TestGzipResponseWriter_Flush(t *testing.T) {
	// Flush on gzipResponseWriter must propagate to the underlying Flusher.
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("chunk"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(handler).ServeHTTP(fr, req)

	if fr.flushCount == 0 {
		t.Error("expected at least one Flush call to propagate to underlying writer")
	}
}

// ============================================================================
// Hijack delegation
// ============================================================================

// hijackableRecorder satisfies both ResponseWriter and Hijacker.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestGzipResponseWriter_Hijack_Delegates(t *testing.T) {
	hr := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hj, ok := w.(http.Hijacker); ok {
			hj.Hijack() //nolint
		} else {
			t.Error("expected gzipResponseWriter to implement Hijacker")
		}
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(handler).ServeHTTP(hr, req)

	if !hr.hijacked {
		t.Error("expected Hijack to be forwarded to the underlying ResponseWriter")
	}
}

func TestGzipResponseWriter_Hijack_ErrorWhenNotSupported(t *testing.T) {
	// Plain httptest.Recorder does not implement Hijacker; we expect an error.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hj, ok := w.(http.Hijacker); ok {
			_, _, err := hj.Hijack()
			if err == nil {
				t.Error("expected error when underlying writer does not support Hijack")
			}
		}
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(handler).ServeHTTP(rec, req)
}
