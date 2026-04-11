package golitekit

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

// CompressionMiddleware compresses responses with gzip when the client accepts it.
// level is optional; defaults to gzip.DefaultCompression.
func CompressionMiddleware(level ...int) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				return next(ctx, w, r)
			}

			l := gzip.DefaultCompression
			if len(level) > 0 {
				l = level[0]
			}

			gz, err := gzip.NewWriterLevel(w, l)
			if err != nil {
				return ErrInternal("gzip init failed", err)
			}

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			w.Header().Del("Content-Length")

			gzw := &gzipResponseWriter{
				ResponseWriter: w,
				Writer:         gz,
			}

			err = next(ctx, gzw, r)

			// Close flushes remaining data; errors may truncate the client response.
			if closeErr := gz.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "gzip close error: %v\n", closeErr)
			}

			return err
		}
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

// Flush flushes the gzip buffer first, then the underlying connection.
func (w *gzipResponseWriter) Flush() {
	if gz, ok := w.Writer.(*gzip.Writer); ok {
		if err := gz.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "gzip flush error: %v\n", err)
			return
		}
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards WebSocket / HTTP upgrade requests to the underlying ResponseWriter.
func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}
