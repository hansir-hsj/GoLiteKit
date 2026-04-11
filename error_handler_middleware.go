package golitekit

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
)

type errorHandlerConfig struct {
	formatter func(w http.ResponseWriter, err *AppError, logID string)
	onError   func(r *http.Request, err *AppError)
	onPanic   func(r *http.Request, recovered any)
}

type ErrorHandlerOption func(*errorHandlerConfig)

// WithErrorFormatter sets a custom function to render AppError responses.
func WithErrorFormatter(f func(w http.ResponseWriter, err *AppError, logID string)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.formatter = f
	}
}

// WithErrorCallback sets a hook called after every AppError is handled.
func WithErrorCallback(f func(r *http.Request, err *AppError)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.onError = f
	}
}

// WithPanicCallback sets a hook called when a panic is recovered.
func WithPanicCallback(f func(r *http.Request, recovered any)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.onPanic = f
	}
}

// deferredWriterPool reuses deferredResponseWriter allocations across requests.
var deferredWriterPool = sync.Pool{
	New: func() any {
		return &deferredResponseWriter{
			header:     make(http.Header),
			statusCode: http.StatusOK,
		}
	},
}

// ErrorHandlerMiddleware is the outermost middleware. It catches errors returned
// by inner handlers and panics, writing appropriate JSON responses.
func ErrorHandlerMiddleware(opts ...ErrorHandlerOption) Middleware {
	cfg := &errorHandlerConfig{
		formatter: defaultErrorFormatter,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			dw := deferredWriterPool.Get().(*deferredResponseWriter)
			dw.ResponseWriter = w

			defer func() {
				dw.resetForPool()
				deferredWriterPool.Put(dw)
			}()

			defer func() {
				if p := recover(); p != nil {
					dw.Reset()
					handlePanic(w, r, p, cfg)
				}
			}()

			err := next(ctx, dw, r)

			if err != nil && !dw.IsFlushed() {
				dw.Reset()
				handleAppError(w, r, WrapError(err, http.StatusInternalServerError), cfg)
				return nil
			}

			dw.Commit()
			return nil
		}
	}
}

// handlePanic handles panic and returns 500 error.
func handlePanic(w http.ResponseWriter, r *http.Request, recovered any, cfg *errorHandlerConfig) {
	ctx := r.Context()
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	if gcx := GetContext(ctx); gcx != nil && gcx.PanicLogger() != nil {
		gcx.PanicLogger().Report(ctx, recovered)
	}

	if cfg.onPanic != nil {
		cfg.onPanic(r, recovered)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)

	resp := Response{
		Status: http.StatusInternalServerError,
		Msg:    "Internal Server Error",
		LogID:  logID,
	}
	json.NewEncoder(w).Encode(resp)
}

// handleAppError handles business errors.
func handleAppError(w http.ResponseWriter, r *http.Request, err *AppError, cfg *errorHandlerConfig) {
	ctx := r.Context()
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	if cfg.onError != nil {
		cfg.onError(r, err)
	}

	cfg.formatter(w, err, logID)
}

// defaultErrorFormatter formats error as JSON response.
func defaultErrorFormatter(w http.ResponseWriter, err *AppError, logID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(err.Code)

	resp := Response{
		Status: err.Code,
		Msg:    err.Message,
		LogID:  logID,
	}

	json.NewEncoder(w).Encode(resp)
}
