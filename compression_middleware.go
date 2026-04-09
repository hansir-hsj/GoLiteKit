package golitekit

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

// CompressionMiddleware compresses responses with gzip when the client accepts it.
// level is optional; defaults to gzip.DefaultCompression.
func CompressionMiddleware(level ...int) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			l := gzip.DefaultCompression
			if len(level) > 0 {
				l = level[0]
			}

			gz, err := gzip.NewWriterLevel(w, l)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			w.Header().Del("Content-Length")

			gzw := &gzipResponseWriter{
				ResponseWriter: w,
				Writer:         gz,
			}

			next.ServeHTTP(gzw, r)

		// Close flushes remaining data into the gzip stream.
		// Errors here may truncate the client response; log to stderr
		// because the response header is already committed.
			if err := gz.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "gzip close error: %v\n", err)
			}
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	io.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", http.DetectContentType(b))
	}
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if statusCode == http.StatusNoContent || statusCode == http.StatusNotModified {
		w.Header().Del("Content-Encoding")
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// Flush flushes the gzip buffer first, then the underlying connection,
// so streaming responses (e.g. SSE) are delivered to the client promptly.
func (w *gzipResponseWriter) Flush() {
	if gz, ok := w.Writer.(*gzip.Writer); ok {
		// Flush compresses and writes buffered data without closing the stream.
		if err := gz.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "gzip flush error: %v\n", err)
			return
		}
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards WebSocket / HTTP upgrade requests to the underlying
// ResponseWriter.  Without this, any WebSocket upgrade through the gzip
// middleware would fail with "ResponseWriter does not implement Hijacker".
func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}
