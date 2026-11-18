package golitekit

import (
	"context"
	"errors"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

// DefaultLogBodyLimit is the max bytes logged for request/response bodies.
const DefaultLogBodyLimit int64 = 4096

// LoggerOptions configures the logger middleware.
type LoggerOptions struct {
	LogRequestBody  bool
	LogResponseBody bool
	MaxBodyBytes    int64
}

// LoggerAsMiddleware logs each request with logInst; panicInst may be nil.
// Place it outside ErrorHandlerMiddleware so it logs the status actually sent.
func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger, opts ...LoggerOptions) Middleware {
	var opt LoggerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.MaxBodyBytes <= 0 {
		opt.MaxBodyBytes = DefaultLogBodyLimit
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) (rerr error) {
			gcx := GetContext(ctx)
			logReqBody := opt.LogRequestBody
			logRespBody := opt.LogResponseBody

			rw := newResponseCapture(w, logRespBody, opt.MaxBodyBytes)
			completed := false
			defer func() {
				if logReqBody && r.Method != http.MethodGet && r.Method != http.MethodDelete {
					if gcx != nil && len(gcx.rawBody) > 0 && isLoggableContentType(r.Header.Get("Content-Type")) {
						logger.AddInfo(ctx, "request", sanitizeLoggedBody(gcx.rawBody, opt.MaxBodyBytes, r.Header.Get("Content-Type")))
					}
				}

				if logRespBody {
					if body, total := rw.capturedBody(); len(body) > 0 {
						logger.AddInfo(ctx, "response", sanitizeCapturedBody(body, total, opt.MaxBodyBytes, rw.Header().Get("Content-Type")))
					}
				}

				if !completed {
					// A panic is propagating: no inner middleware recovered it.
					// Do not recover here; just record the request.
					logger.AddInfo(ctx, "status", http.StatusInternalServerError)
					if logInst != nil {
						logInst.Error(ctx, "request panicked")
					}
					return
				}

				reqErr := rerr
				if reqErr == nil {
					reqErr = HandledError(ctx)
				}
				status := rw.status()
				if reqErr != nil && !rw.headerWritten() {
					// The error response will be written by an outer layer.
					status = WrapError(reqErr, http.StatusInternalServerError).Code
				}
				logger.AddInfo(ctx, "status", status)

				if reqErr == nil {
					if logInst != nil {
						logInst.Info(ctx, "succ")
					}
					return
				}

				var appErr *AppError
				if errors.As(reqErr, &appErr) {
					logger.AddInfo(ctx, "err_code", appErr.Code)
					if appErr.Status != 0 {
						logger.AddInfo(ctx, "biz_status", appErr.Status)
					}
					logger.AddInfo(ctx, "err_message", appErr.Message)
					if appErr.Internal != nil {
						logger.AddInfo(ctx, "err_internal", sanitizeErrorMessage(appErr.Internal.Error(), opt.MaxBodyBytes))
					}
				} else {
					logger.AddInfo(ctx, "err_internal", sanitizeErrorMessage(reqErr.Error(), opt.MaxBodyBytes))
				}
				if logInst == nil {
					return
				}
				if status >= http.StatusInternalServerError {
					logInst.Error(ctx, "request completed with error")
				} else {
					logInst.Warning(ctx, "request completed with error")
				}
			}()

			if gcx != nil {
				gcx.responseWriter = rw
				gcx.setContextOptions(withLogger(logInst), withPanicLogger(panicInst))
			}

			logger.AddInfo(ctx, "method", r.Method)
			if gcx != nil && gcx.request != nil && gcx.request.URL == r.URL {
				// Reuse the Context's cached query parse.
				logger.AddInfo(ctx, "url", sanitizeURLWithQuery(r.URL.Path, gcx.queryValues()))
			} else {
				logger.AddInfo(ctx, "url", sanitizeURL(r.URL))
			}

			rerr = next(ctx, rw, r)
			completed = true
			return
		}
	}
}
