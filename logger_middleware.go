package golitekit

import (
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithContext(r.Context())
			gcx := GetContext(ctx)

			rw := newResponseCapture(w)
			gcx.responseWriter = rw

			gcx.SetContextOptions(withLogger(logInst), withPanicLogger(panicInst))

			logger.AddInfo(ctx, "method", r.Method)
			logger.AddInfo(ctx, "url", r.URL.String())

			next.ServeHTTP(rw, r)

			if env.LogRequestBody() && r.Method != http.MethodGet && r.Method != http.MethodDelete {
				if len(gcx.RawBody) > 0 {
					logger.AddInfo(ctx, "request", string(gcx.RawBody))
				}
			}

			if env.LogResponseBody() && len(rw.body) > 0 {
				logger.AddInfo(ctx, "response", string(rw.body))
			}

			logger.AddInfo(ctx, "status", rw.statusCode)

			if appErr := GetError(ctx); appErr != nil {
				logger.AddInfo(ctx, "err_code", appErr.Code)
				logger.AddInfo(ctx, "err_message", appErr.Message)
				if appErr.Internal != nil {
					logger.AddInfo(ctx, "err_internal", appErr.Internal.Error())
				}
				logInst.Warning(ctx, "request completed with error")
			} else {
				logInst.Info(ctx, "succ")
			}

		})
	}
}
