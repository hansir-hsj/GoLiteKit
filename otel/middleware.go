package otel

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"github.com/hansir-hsj/GoLiteKit/logger"
	"go.opentelemetry.io/otel/trace"
)

func Middleware(observer *Observer, opts ...Option) glk.Middleware {
	options := applyOptions(opts)

	return func(next glk.Handler) glk.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			pattern := routePattern(r)
			spanName := "HTTP " + r.Method + " " + pattern
			ctx = glk.WithObserverContext(ctx, observer)
			ctx, span := observer.StartSpan(ctx, spanName,
				glk.StringAttr("http.request.method", r.Method),
				glk.StringAttr("http.route", pattern),
			)
			defer span.End()

			if traceSpan := trace.SpanFromContext(ctx); traceSpan != nil {
				spanCtx := traceSpan.SpanContext()
				if spanCtx.HasTraceID() {
					logger.AddInfo(ctx, "trace_id", spanCtx.TraceID().String())
				}
				if spanCtx.HasSpanID() {
					logger.AddInfo(ctx, "span_id", spanCtx.SpanID().String())
				}
			}

			started := time.Now()
			cw := &statusCapture{ResponseWriter: w, statusCode: http.StatusOK}
			err := next(ctx, cw, r.WithContext(ctx))
			status := cw.statusCode

			span.SetAttributes(
				glk.IntAttr("http.response.status_code", status),
				glk.FloatAttr("http.server.duration_ms", float64(time.Since(started))/float64(time.Millisecond)),
			)

			if err != nil {
				span.SetError(err)
			} else if status >= http.StatusInternalServerError || (options.ClientErrorAsSpanError && status >= http.StatusBadRequest) {
				span.SetStatus(glk.SpanStatusError, http.StatusText(status))
			} else {
				span.SetStatus(glk.SpanStatusOK, http.StatusText(status))
			}

			return err
		}
	}
}

func routePattern(r *http.Request) string {
	if r.Pattern == "" {
		return r.URL.Path
	}
	if strings.HasPrefix(r.Pattern, r.Method+" ") {
		return strings.TrimPrefix(r.Pattern, r.Method+" ")
	}
	return r.Pattern
}

type statusCapture struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func (w *statusCapture) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.headerWritten = true
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusCapture) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusCapture) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}
