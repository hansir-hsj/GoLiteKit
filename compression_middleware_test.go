package golitekit

import (
	"bufio"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCompressionMiddleware_CompressesWhenAccepted(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte("hello compressed world"))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}

	body := gunzip(t, rec.Body.Bytes())
	if body != "hello compressed world" {
		t.Errorf("decompressed body = %q, want %q", body, "hello compressed world")
	}
}

func TestCompressionMiddleware_SkipsWithoutAcceptEncoding(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte("plain text"))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip when Accept-Encoding is absent")
	}
	if rec.Body.String() != "plain text" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "plain text")
	}
}

func TestCompressionMiddleware_VaryHeader(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte("data"))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
	}
}

func TestCompressionMiddleware_NoContentLength(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte("data"))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
}

func TestCompressionMiddleware_NoContent204(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected Content-Encoding to be removed for 204 No Content")
	}
}

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
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte("chunk"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(inner).ServeHTTP(fr, req)

	if fr.flushCount == 0 {
		t.Error("expected at least one Flush call to propagate to underlying writer")
	}
}

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

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if hj, ok := w.(http.Hijacker); ok {
			hj.Hijack() //nolint
		} else {
			t.Error("expected gzipResponseWriter to implement Hijacker")
		}
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(inner).ServeHTTP(hr, req)

	if !hr.hijacked {
		t.Error("expected Hijack to be forwarded to the underlying ResponseWriter")
	}
}

func TestGzipResponseWriter_Hijack_ErrorWhenNotSupported(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if hj, ok := w.(http.Hijacker); ok {
			_, _, err := hj.Hijack()
			if err == nil {
				t.Error("expected error when underlying writer does not support Hijack")
			}
		}
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)
}
