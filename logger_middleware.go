package golitekit

import (
	"context"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// LoggerAsMiddleware logs each request and its outcome using logInst.
// panicInst is optional; pass nil to skip panic logging.
func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) (rerr error) {
			gcx := GetContext(ctx)

			rw := newResponseCapture(w)
			defer func() {
				if env.LogRequestBody() && r.Method != http.MethodGet && r.Method != http.MethodDelete {
					if gcx != nil && len(gcx.RawBody) > 0 {
						logger.AddInfo(ctx, "request", string(gcx.RawBody))
					}
				}

				if env.LogResponseBody() && len(rw.body) > 0 {
					logger.AddInfo(ctx, "response", string(rw.body))
				}

				logger.AddInfo(ctx, "status", rw.statusCode)

				if rerr != nil {
					if appErr, ok := rerr.(*AppError); ok {
						logger.AddInfo(ctx, "err_code", appErr.Code)
						logger.AddInfo(ctx, "err_message", appErr.Message)
						if appErr.Internal != nil {
							logger.AddInfo(ctx, "err_internal", appErr.Internal.Error())
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

				rw.resetForPool()
				responseCapturePool.Put(rw)
			}()

			if gcx != nil {
				gcx.responseWriter = rw
				gcx.SetContextOptions(withLogger(logInst), withPanicLogger(panicInst))
			}

			logger.AddInfo(ctx, "method", r.Method)
			logger.AddInfo(ctx, "url", r.URL.String())

			rerr = next(ctx, rw, r)
			return
		}
	}
}
