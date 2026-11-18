package golitekit

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// DefaultCompressionMinSize is the smallest body CompressionMiddleware compresses.
const DefaultCompressionMinSize = 1024

// CompressionMiddleware gzips responses when the client accepts it. level
// defaults to gzip.DefaultCompression.
//
// Skipped: HEAD, 1xx/204/304, 206, already encoded or compressed types, and
// bodies under DefaultCompressionMinSize. Strong ETags become weak.
func CompressionMiddleware(level ...int) Middleware {
	l := gzip.DefaultCompression
	if len(level) > 0 {
		l = level[0]
	}
	validLevel := l >= gzip.HuffmanOnly && l <= gzip.BestCompression
	pool := &sync.Pool{New: func() any {
		gz, _ := gzip.NewWriterLevel(io.Discard, l) // only used when validLevel
		return gz
	}}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			if r.Method == http.MethodHead || !acceptsGzip(r.Header.Values("Accept-Encoding")) {
				return next(ctx, w, r)
			}
			if !validLevel {
				return ErrInternal("gzip init failed", fmt.Errorf("gzip: invalid compression level: %d", l))
			}

			gzw := &gzipResponseWriter{ResponseWriter: w, pool: pool}
			err := next(ctx, gzw, r)

			// Close flushes remaining data; errors may truncate the client response.
			if closeErr := gzw.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "gzip close error: %v\n", closeErr)
			}

			return err
		}
	}
}

// acceptsGzip reports whether Accept-Encoding allows gzip (RFC 9110 12.5.3):
// "gzip" with q > 0, or "*" with q > 0 when gzip is not listed explicitly.
func acceptsGzip(headers []string) bool {
	gzipQ, starQ := -1.0, -1.0
	for _, h := range headers {
		for _, part := range strings.Split(h, ",") {
			coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			q, ok := qValue(params)
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(coding)) {
			case "gzip", "x-gzip":
				gzipQ = q
			case "*":
				starQ = q
			}
		}
	}
	if gzipQ >= 0 {
		return gzipQ > 0
	}
	return starQ > 0
}

// qValue returns the q parameter among ";"-separated params (1 if absent).
// ok is false when q is malformed, so the coding is ignored.
func qValue(params string) (q float64, ok bool) {
	q = 1
	for _, p := range strings.Split(params, ";") {
		k, v, found := strings.Cut(strings.TrimSpace(p), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(k), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		q = parsed
	}
	return q, true
}

// incompressible reports whether a body of this Content-Type is already
// compressed or binary enough that gzip would not help.
func incompressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case mt == "image/svg+xml":
		return false
	case strings.HasPrefix(mt, "image/"), strings.HasPrefix(mt, "video/"),
		strings.HasPrefix(mt, "audio/"), strings.HasPrefix(mt, "font/"):
		return true
	}
	switch mt {
	case "application/octet-stream", "application/zip", "application/gzip",
		"application/x-gzip", "application/zstd", "application/x-bzip2",
		"application/x-xz", "application/x-7z-compressed", "application/x-rar-compressed",
		"application/pdf", "application/wasm":
		return true
	}
	return false
}

// gzipResponseWriter holds the status line and the first
// DefaultCompressionMinSize body bytes until it knows whether to compress.
type gzipResponseWriter struct {
	http.ResponseWriter
	pool *sync.Pool

	status  int    // pending status; 0 = not set
	buf     []byte // body written before the decision
	decided bool   // status line and headers have been sent
	gz      *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.decided {
		return
	}
	if statusCode >= 100 && statusCode < 200 && statusCode != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(statusCode) // informational; final status follows
		return
	}
	if w.status != 0 {
		return
	}
	w.status = statusCode
	if statusCode == http.StatusNoContent || statusCode == http.StatusNotModified {
		w.decide(false) // no body
	}
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.decided {
		if w.gz != nil {
			return w.gz.Write(b)
		}
		return w.ResponseWriter.Write(b)
	}
	w.buf = append(w.buf, b...)
	if len(w.buf) >= DefaultCompressionMinSize {
		if err := w.decide(true); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

// decide sends the status line and headers, compressing the body if
// compress is set and the response is eligible, then writes the buffer.
func (w *gzipResponseWriter) decide(compress bool) error {
	w.decided = true
	if w.status == 0 {
		w.status = http.StatusOK
	}
	h := w.Header()
	if h.Get("Content-Type") == "" && len(w.buf) > 0 {
		h.Set("Content-Type", http.DetectContentType(w.buf))
	}
	compress = compress && w.status != http.StatusPartialContent && h.Get("Content-Range") == "" &&
		h.Get("Content-Encoding") == "" && !incompressible(h.Get("Content-Type"))
	if compress {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		if !headerHasToken(h, "Vary", "Accept-Encoding") {
			h.Add("Vary", "Accept-Encoding")
		}
		// A strong ETag names the exact bytes; the gzip body differs.
		if etag := h.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
			h.Set("ETag", "W/"+etag)
		}
		w.gz = w.pool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(w.status)

	buf := w.buf
	w.buf = nil
	if len(buf) == 0 {
		return nil
	}
	var err error
	if w.gz != nil {
		_, err = w.gz.Write(buf)
	} else {
		_, err = w.ResponseWriter.Write(buf)
	}
	return err
}

// Close finishes the response. If the handler wrote neither a status nor a
// body, nothing is sent.
func (w *gzipResponseWriter) Close() error {
	if !w.decided {
		if w.status == 0 && len(w.buf) == 0 {
			return nil
		}
		if err := w.decide(len(w.buf) >= DefaultCompressionMinSize); err != nil {
			return err
		}
	}
	if w.gz == nil {
		return nil
	}
	err := w.gz.Close()
	// Drop the reference to this response before pooling the writer.
	w.gz.Reset(io.Discard)
	w.pool.Put(w.gz)
	w.gz = nil
	return err
}

// Flush sends what has been written so far, deciding on compression now if
// needed, then flushes the underlying connection.
func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		if err := w.decide(len(w.buf) >= DefaultCompressionMinSize); err != nil {
			fmt.Fprintf(os.Stderr, "gzip write error: %v\n", err)
			return
		}
	}
	if w.gz != nil {
		if err := w.gz.Flush(); err != nil {
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
		w.decided = true // the connection is no longer ours to finish
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// headerHasToken reports whether a comma-separated header contains token.
func headerHasToken(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
