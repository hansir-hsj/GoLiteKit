package golitekit

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

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
			defer gz.Close()

			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			// Remove Content-Length to avoid issues with compressed content
			w.Header().Del("Content-Length")

			gzw := &gzipResponseWriter{
				ResponseWriter: w,
				Writer:         gz,
			}

			next.ServeHTTP(gzw, r)
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
