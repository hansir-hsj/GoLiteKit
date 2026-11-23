package golitekit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
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

// ErrorHandlerMiddleware turns returned errors and panics into JSON error
// responses and returns nil; outer middlewares read the error via HandledError.
func ErrorHandlerMiddleware(opts ...ErrorHandlerOption) Middleware {
	cfg := &errorHandlerConfig{
		formatter: defaultErrorFormatter,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			dw := newDeferredResponseWriter(w)

			defer func() {
				if p := recover(); p != nil {
					// http.ErrAbortHandler asks net/http to abort the response
					// (e.g. httputil.ReverseProxy); it is not a bug to report.
					if p == http.ErrAbortHandler {
						panic(p)
					}
					// Once part of the response has been sent, only report the
					// panic; writing a 500 now would corrupt the body.
					writeResponse := !dw.responseSent()
					dw.Reset()
					handlePanic(w, r, p, cfg, writeResponse)
				}
			}()

			err := next(ctx, dw, r)

			if err != nil {
				writeResponse := !dw.responseSent()
				dw.Reset()
				handleAppError(w, r, WrapError(err, http.StatusInternalServerError), cfg, writeResponse)
				return nil
			}

			dw.Commit()
			return nil
		}
	}
}

// handlePanic reports a recovered panic and, if writeResponse is set, writes a 500 error.
func handlePanic(w http.ResponseWriter, r *http.Request, recovered any, cfg *errorHandlerConfig, writeResponse bool) {
	ctx := r.Context()
	logID := EnsureLogID(ctx)

	gcx := GetContext(ctx)
	if gcx != nil {
		gcx.setHandledError(ErrInternal("Internal Server Error", panicError{value: recovered}))
		if gcx.PanicLogger() != nil {
			gcx.PanicLogger().Report(ctx, recovered)
		} else {
			// No panic logger configured: still record the stack.
			fmt.Fprintf(os.Stderr, "golitekit: panic: %v\n%s\n", recovered, debug.Stack())
		}
	}

	if cfg.onPanic != nil {
		cfg.onPanic(r, recovered)
	}

	if !writeResponse {
		return
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

// handleAppError reports a business error and, if writeResponse is set, renders it.
func handleAppError(w http.ResponseWriter, r *http.Request, err *AppError, cfg *errorHandlerConfig, writeResponse bool) {
	ctx := r.Context()
	logID := EnsureLogID(ctx)

	if gcx := GetContext(ctx); gcx != nil {
		gcx.setHandledError(err)
	}

	if cfg.onError != nil {
		cfg.onError(r, err)
	}

	if !writeResponse {
		return
	}

	cfg.formatter(w, err, logID)
}

// defaultErrorFormatter writes err as the JSON envelope with HTTP status
// err.Code and "status" set to err.ResponseStatus().
func defaultErrorFormatter(w http.ResponseWriter, err *AppError, logID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(err.Code)

	resp := Response{
		Status: err.ResponseStatus(),
		Msg:    err.Message,
		LogID:  logID,
	}

	json.NewEncoder(w).Encode(resp)
}
