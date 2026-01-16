package golitekit

import (
	"encoding/json"
	"net/http"
)

type errorHandlerConfig struct {
	formatter func(w http.ResponseWriter, err *AppError, logID string)
	onError   func(r *http.Request, err *AppError)
	onPanic   func(r *http.Request, recovered any)
}

type ErrorHandlerOption func(*errorHandlerConfig)

func WithErrorFormatter(f func(w http.ResponseWriter, err *AppError, logID string)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.formatter = f
	}
}

func WithErrorCallback(f func(r *http.Request, err *AppError)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.onError = f
	}
}

func WithPanicCallback(f func(r *http.Request, recovered any)) ErrorHandlerOption {
	return func(c *errorHandlerConfig) {
		c.onPanic = f
	}
}

// ErrorHandlerMiddleware unified error handling middleware
// it should be placed at the outermost layer of middleware chain
func ErrorHandlerMiddleware(opts ...ErrorHandlerOption) HandlerMiddleware {
	cfg := &errorHandlerConfig{
		formatter: defaultErrorFormatter,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ResponseWriterwith deferred writing
			dw := newDeferredResponseWriter(w)

			// panic-independent error process path
			defer func() {
				if p := recover(); p != nil {
					handlePanic(w, r, p, cfg)
				}
			}()

			// process handler chain
			next.ServeHTTP(dw, r)

			ctx := r.Context()

			// check error
			if appErr := GetError(ctx); appErr != nil {
				// reset
				dw.Reset()
				// call unified error handling
				handleAppError(w, r, appErr, cfg)
				return
			}

			// no errors, submit response
			dw.Commit()
		})
	}
}

// handlePanic specifically handle panic
func handlePanic(w http.ResponseWriter, r *http.Request, recovered any, cfg *errorHandlerConfig) {
	ctx := r.Context()
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	// 1. record panic logs - including the complete stack trace
	if gcx := GetContext(ctx); gcx != nil && gcx.PanicLogger() != nil {
		gcx.PanicLogger().Report(ctx, recovered)
	}

	// 2. trigger panic-specific callbaks
	if cfg.onPanic != nil {
		cfg.onPanic(r, recovered)
	}

	// 3. return 500 error using unified Response format
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)

	resp := Response{
		Status: http.StatusInternalServerError,
		Msg:    "Internal Server Error",
		LogID:  logID,
	}
	json.NewEncoder(w).Encode(resp)
}

// handleAppError - handle business error
func handleAppError(w http.ResponseWriter, r *http.Request, err *AppError, cfg *errorHandlerConfig) {
	ctx := r.Context()
	logID := ""
	if tracker := GetTracker(ctx); tracker != nil {
		logID = tracker.LogID()
	}

	// business error callback
	if cfg.onError != nil {
		cfg.onError(r, err)
	}

	cfg.formatter(w, err, logID)
}

// defaultErrorFormatter using unified Response format
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
