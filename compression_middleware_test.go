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
	"time"
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

// bigBody is above DefaultCompressionMinSize, so it gets compressed.
var bigBody = strings.Repeat("hello compressed world ", 100)

func TestCompressionMiddleware_CompressesWhenAccepted(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte(bigBody))
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
	if body != bigBody {
		t.Errorf("decompressed body length = %d, want %d", len(body), len(bigBody))
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
		w.Header().Set("Vary", "Origin")
		w.Write([]byte(bigBody))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	// The existing Vary value is kept and Accept-Encoding is added.
	if got := rec.Header().Values("Vary"); len(got) != 2 || got[0] != "Origin" || got[1] != "Accept-Encoding" {
		t.Errorf("Vary = %q, want [Origin Accept-Encoding]", got)
	}
}

func TestCompressionMiddleware_NoContentLength(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Length", "2300")
		w.Write([]byte(bigBody))
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	mw(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != "" {
		t.Errorf("Content-Encoding = %q Content-Length = %q", rec.Header().Get("Content-Encoding"), rec.Header().Get("Content-Length"))
	}
}

func TestCompressionMiddleware_SkipsIneligibleResponses(t *testing.T) {
	cases := []struct {
		name           string
		method         string
		acceptEncoding string
		setup          func(w http.ResponseWriter)
		body           string
	}{
		{"small body", http.MethodGet, "gzip", nil, "tiny"},
		{"q=0", http.MethodGet, "gzip;q=0, deflate", nil, bigBody},
		{"star with gzip excluded", http.MethodGet, "*, gzip;q=0", nil, bigBody},
		{"HEAD", http.MethodHead, "gzip", nil, ""},
		{"image", http.MethodGet, "gzip", func(w http.ResponseWriter) { w.Header().Set("Content-Type", "image/png") }, bigBody},
		{"already encoded", http.MethodGet, "gzip", func(w http.ResponseWriter) { w.Header().Set("Content-Encoding", "br") }, bigBody},
		{"partial content", http.MethodGet, "gzip", func(w http.ResponseWriter) {
			w.Header().Set("Content-Range", "bytes 0-4095/8192")
			w.WriteHeader(http.StatusPartialContent)
		}, bigBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
				if tc.setup != nil {
					tc.setup(w)
				}
				w.Write([]byte(tc.body))
				return nil
			})
			req := httptest.NewRequest(tc.method, "/", nil)
			req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			rec := httptest.NewRecorder()
			CompressionMiddleware()(inner).ServeHTTP(rec, req)

			if rec.Header().Get("Content-Encoding") == "gzip" {
				t.Fatal("response should not be gzip-compressed")
			}
			if rec.Body.String() != tc.body {
				t.Fatalf("body changed: %.40q", rec.Body.String())
			}
		})
	}
}

func BenchmarkCompressionMiddleware(b *testing.B) {
	h := CompressionMiddleware()(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Write([]byte(bigBody))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"gzip":               true,
		"gzip, deflate, br":  true,
		"GZIP;q=0.5":         true,
		"gzip;q=0":           false,
		"gzip; q=0.0":        false,
		"deflate":            false,
		"*":                  true,
		"*;q=0":              false,
		"br, *;q=0.1":        true,
		"identity, gzip;q=0": false,
		"gzip;foo=1;q=0":     false,
		"gzip;q=abc":         false,
	} {
		if got := acceptsGzip([]string{header}); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestCompressionMiddleware_WeakensStrongETag(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(bigBody))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	CompressionMiddleware()(inner).ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("ETag") != `W/"v1"` {
		t.Fatalf("Content-Encoding = %q, ETag = %q", rec.Header().Get("Content-Encoding"), rec.Header().Get("ETag"))
	}
}

func TestCompressionMiddleware_SmallFlushedStreamIsNotCompressed(t *testing.T) {
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: 1\n\n"))
		w.(http.Flusher).Flush()
		w.Write([]byte("data: 2\n\n"))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	CompressionMiddleware()(inner).ServeHTTP(fr, req)

	if fr.flushCount == 0 || fr.Header().Get("Content-Encoding") != "" {
		t.Fatalf("flushes = %d Content-Encoding = %q", fr.flushCount, fr.Header().Get("Content-Encoding"))
	}
	if fr.Body.String() != "data: 1\n\ndata: 2\n\n" {
		t.Fatalf("body = %q", fr.Body.String())
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
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for 204 No Content, got %q", rec.Body.String())
	}
}

func TestCompressionMiddleware_ErrorWithoutWriteIsNotCompressed(t *testing.T) {
	h := ErrorHandlerMiddleware()(CompressionMiddleware()(
		func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			return ErrBadRequest("bad", nil)
		}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req = req.WithContext(withContext(req.Context()))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding = %q, want none on plain error response", rec.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(rec.Body.String(), `"msg":"bad"`) {
		t.Fatalf("body = %q, want JSON error", rec.Body.String())
	}
}

func TestCompressionMiddleware_ExplicitStatusThenWrite(t *testing.T) {
	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(bigBody))
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()

	CompressionMiddleware()(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("status = %d, Content-Encoding = %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	if body := gunzip(t, rec.Body.Bytes()); body != bigBody {
		t.Fatalf("decompressed body length = %d, want %d", len(body), len(bigBody))
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

func TestGzipResponseWriter_ResponseControllerFlush(t *testing.T) {
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if _, err := w.Write([]byte("chunk")); err != nil {
			return err
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Fatalf("ResponseController.Flush: %v", err)
		}
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(inner).ServeHTTP(fr, req)

	if fr.flushCount == 0 {
		t.Fatal("expected flush to reach underlying writer")
	}
}

func TestGzipResponseWriter_ResponseControllerSetWriteDeadline(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	deadline := time.Now().Add(time.Second)

	inner := Handler(func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil {
			t.Fatalf("ResponseController.SetWriteDeadline: %v", err)
		}
		return nil
	})

	mw := CompressionMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	mw(inner).ServeHTTP(rec, req)

	if !rec.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", rec.writeDeadline, deadline)
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
