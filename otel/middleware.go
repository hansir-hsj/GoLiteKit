package otel

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	glk "github.com/hansir-hsj/GoLiteKit"
	"github.com/hansir-hsj/GoLiteKit/logger"
	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Middleware starts a server span per request, joining the incoming trace,
// and records HTTP metrics.
func Middleware(observer *Observer, opts ...Option) glk.Middleware {
	options := applyOptions(opts)
	propagator := options.Propagator
	if propagator == nil {
		propagator = otelapi.GetTextMapPropagator()
	}

	return func(next glk.Handler) glk.Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) (err error) {
			// Route is the low-cardinality path template; it is empty when the
			// request did not match a route, and is then omitted.
			route := routePattern(r)
			method, known := normalizeMethod(r.Method)
			spanName := "HTTP"
			if known {
				spanName += " " + method
			}
			if route != "" {
				spanName += " " + route
			}
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))
			// The router already exposes its observer through the request
			// Context; only add a layer when it is a different one.
			if glk.ObserverFromContext(ctx) != glk.Observer(observer) {
				ctx = glk.WithObserverContext(ctx, observer)
			}
			spanAttrs := []glk.Attribute{
				glk.StringAttr("http.request.method", method),
				glk.StringAttr("url.scheme", scheme),
			}
			if !known {
				spanAttrs = append(spanAttrs, glk.StringAttr("http.request.method_original", r.Method))
			}
			if route != "" {
				spanAttrs = append(spanAttrs, glk.StringAttr("http.route", route))
			}
			ctx, traceSpan := observer.tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(mapAttributes(spanAttrs)...),
			)
			span := &spanImpl{
				ctx:     ctx,
				span:    traceSpan,
				metrics: observer.metrics,
				name:    spanName,
				started: time.Now(),
				attrs:   spanAttrs,
			}
			defer span.End()

			spanCtx := traceSpan.SpanContext()
			if spanCtx.HasTraceID() {
				logger.AddInfo(ctx, "trace_id", spanCtx.TraceID().String())
			}
			if spanCtx.HasSpanID() {
				logger.AddInfo(ctx, "span_id", spanCtx.SpanID().String())
			}

			started := time.Now()
			cw := &statusCapture{ResponseWriter: w, statusCode: http.StatusOK}

			finish := func(err error, panicked bool) {
				elapsed := time.Since(started)

				// ErrorHandlerMiddleware (inside this middleware in the default
				// stack) writes the error response and returns nil.
				reqErr := err
				if reqErr == nil {
					reqErr = glk.HandledError(ctx)
				}
				status := cw.statusCode
				errCode := 0 // status the error stands for, even if not sent
				if reqErr != nil {
					errCode = glk.WrapError(reqErr, http.StatusInternalServerError).Code
				}
				if panicked {
					errCode = http.StatusInternalServerError
				}
				if (err != nil || panicked) && !cw.headerWritten {
					// Nothing was written yet; an outer layer writes the error.
					status = errCode
				}

				// A failure after the response started (e.g. a broken stream)
				// still counts, though the status sent was a success.
				isError := status >= http.StatusInternalServerError || errCode >= http.StatusInternalServerError ||
					(options.ClientErrorAsSpanError && (status >= http.StatusBadRequest || errCode >= http.StatusBadRequest))
				errorType := ""
				if isError {
					errorType = strconv.Itoa(max(status, errCode))
				}

				attrs := []glk.Attribute{
					glk.IntAttr("http.response.status_code", status),
					glk.FloatAttr("http.server.duration_ms", float64(elapsed)/float64(time.Millisecond)),
				}
				if errorType != "" {
					attrs = append(attrs, glk.StringAttr("error.type", errorType))
				}
				span.SetAttributes(attrs...)
				observer.metrics.recordHTTP(ctx, httpRequestAttrs{
					method: method, route: route, scheme: scheme, status: status, errorType: errorType,
				}, elapsed)

				switch {
				case isError && reqErr != nil:
					span.SetError(reqErr)
				case isError:
					span.SetStatus(glk.SpanStatusError, http.StatusText(status))
				default:
					span.SetStatus(glk.SpanStatusOK, http.StatusText(status))
				}
			}

			// Without an ErrorHandlerMiddleware inside, a panic reaches this
			// middleware: record it, then let it propagate.
			defer func() {
				if p := recover(); p != nil {
					var perr error
					if p != http.ErrAbortHandler {
						perr = fmt.Errorf("panic: %v", p)
					}
					finish(perr, true)
					panic(p)
				}
			}()

			err = next(ctx, cw, r.WithContext(ctx))
			finish(err, false)
			return err
		}
	}
}

// normalizeMethod returns method if it is a standard HTTP method and
// "_OTHER" otherwise, so arbitrary request methods cannot create unbounded
// metric series.
func normalizeMethod(method string) (string, bool) {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method, true
	}
	return "_OTHER", false
}

// routePattern returns the path template of the matched ServeMux pattern
// ("[METHOD ][HOST]/PATH"), or "" when the request was not routed by a mux.
// The raw URL path is never used, to keep metric label cardinality bounded.
func routePattern(r *http.Request) string {
	pattern := r.Pattern
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = strings.TrimLeft(pattern[i+1:], " \t")
	}
	if i := strings.IndexByte(pattern, '/'); i > 0 {
		pattern = pattern[i:]
	}
	return pattern
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
	if statusCode >= 100 && statusCode < 200 && statusCode != http.StatusSwitchingProtocols {
		// Informational (e.g. 103 Early Hints): forward it, the final status follows.
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	w.headerWritten = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusCapture) Flush() {
	// Flushing sends the status line (200 if none was set).
	w.headerWritten = true
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusCapture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err == nil && !w.headerWritten {
			// The handler now speaks its own protocol (e.g. a WebSocket
			// upgrade it answers itself).
			w.headerWritten = true
			w.statusCode = http.StatusSwitchingProtocols
		}
		return conn, rw, err
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func (w *statusCapture) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
