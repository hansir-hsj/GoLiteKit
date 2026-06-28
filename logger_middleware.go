package golitekit

import (
	"context"
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

// LoggerAsMiddleware logs each request and its outcome using logInst.
// panicInst is optional; pass nil to skip panic logging.
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
			defer func() {
				if logReqBody && r.Method != http.MethodGet && r.Method != http.MethodDelete {
					if gcx != nil && len(gcx.rawBody) > 0 && isLoggableContentType(r.Header.Get("Content-Type")) {
						logger.AddInfo(ctx, "request", sanitizeLoggedBody(gcx.rawBody, opt.MaxBodyBytes, r.Header.Get("Content-Type")))
					}
				}

				if logRespBody && len(rw.body) > 0 {
					logger.AddInfo(ctx, "response", sanitizeLoggedBody(rw.body, opt.MaxBodyBytes, rw.Header().Get("Content-Type")))
				}

				logger.AddInfo(ctx, "status", rw.statusCode)

				if rerr != nil {
					if appErr, ok := rerr.(*AppError); ok {
						logger.AddInfo(ctx, "err_code", appErr.Code)
						logger.AddInfo(ctx, "err_message", appErr.Message)
						if appErr.Internal != nil {
							logger.AddInfo(ctx, "err_internal", sanitizeErrorMessage(appErr.Internal.Error(), opt.MaxBodyBytes))
						}
					}
					if logInst != nil {
						logInst.Warning(ctx, "request completed with error")
					}
				} else {
					if logInst != nil {
						logInst.Info(ctx, "succ")
					}
				}

			}()

			if gcx != nil {
				gcx.responseWriter = rw
				gcx.setContextOptions(withLogger(logInst), withPanicLogger(panicInst))
			}

			logger.AddInfo(ctx, "method", r.Method)
			logger.AddInfo(ctx, "url", sanitizeURL(r.URL))

			rerr = next(ctx, rw, r)
			return
		}
	}
}
