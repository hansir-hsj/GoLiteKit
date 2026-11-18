package golitekit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// TimeoutOptions configures the timeout middleware.
type TimeoutOptions struct {
	Duration   time.Duration
	SSETimeout time.Duration
}

// TimeoutMiddleware cancels the handler's context after the timeout (none
// without options). If the handler then returns a deadline error or nothing
// was written, it returns 503.
func TimeoutMiddleware(opts ...TimeoutOptions) Middleware {
	var opt TimeoutOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			timeout := opt.Duration
			sseTimeout := opt.SSETimeout
			acceptsSSE := sseTimeout > 0 && strings.Contains(r.Header.Get("Accept"), "text/event-stream")
			if acceptsSSE {
				timeout = sseTimeout
			}

			if timeout < 1 {
				return next(ctx, w, r)
			}

			timeoutCtx, cancel := context.WithTimeoutCause(
				ctx,
				timeout,
				fmt.Errorf("request timeout after %v", timeout),
			)
			defer cancel()

			tw := &writeTracker{ResponseWriter: w}
			if acceptsSSE {
				// Extend the server write deadline only for a real event
				// stream; the Accept header alone is client controlled.
				tw.beforeStart = func() {
					if strings.HasPrefix(tw.Header().Get("Content-Type"), "text/event-stream") {
						// Unsupported writers are ignored.
						_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(sseTimeout))
					}
				}
			}
			err := next(timeoutCtx, tw, r.WithContext(timeoutCtx))

			if !errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				return err
			}
			// errors.Is also sees through *AppError, which the router wraps
			// controller errors in.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Cause(timeoutCtx)) ||
				(err == nil && !tw.wrote.Load()) {
				return ErrServiceUnavailable("Request timeout", context.Cause(timeoutCtx))
			}
			return err
		}
	}
}

// writeTracker records what the handler wrote: wrote for any status or body,
// bodyStarted for body bytes, flushes and hijacks.
type writeTracker struct {
	http.ResponseWriter
	wrote       atomic.Bool
	bodyStarted atomic.Bool
	status      atomic.Int32 // final status passed to WriteHeader, 0 if none

	// beforeStart, if set, runs once before the status line or first body
	// bytes are passed on.
	beforeStart func()
	started     atomic.Bool
}

func (w *writeTracker) start() {
	if w.beforeStart != nil && w.started.CompareAndSwap(false, true) {
		w.beforeStart()
	}
}

func (w *writeTracker) WriteHeader(code int) {
	if code >= 200 || code == http.StatusSwitchingProtocols {
		w.start()
		w.wrote.Store(true)
		w.status.CompareAndSwap(0, int32(code))
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *writeTracker) Write(b []byte) (int, error) {
	w.start()
	w.wrote.Store(true)
	w.bodyStarted.Store(true)
	return w.ResponseWriter.Write(b)
}

func (w *writeTracker) Flush() {
	w.start()
	w.wrote.Store(true)
	w.bodyStarted.Store(true)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *writeTracker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		conn, rw, err := hj.Hijack()
		if err == nil {
			w.wrote.Store(true)
			w.bodyStarted.Store(true)
		}
		return conn, rw, err
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func (w *writeTracker) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
