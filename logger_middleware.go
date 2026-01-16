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

			// Wrap ResponseWriter to capture response body
			rw := newResponseCapture(w)
			gcx.responseWriter = rw

			gcx.SetContextOptions(WithLogger(logInst), WithPanicLogger(panicInst))

			// Log basic request info
			logger.AddInfo(ctx, "method", r.Method)
			logger.AddInfo(ctx, "url", r.URL.String())

			// Execute next handler (controller will parse body here)
			next.ServeHTTP(rw, r)

			// Log request body after processing (RawBody is now available)
			// Skip GET and DELETE requests as they typically have no body
			if env.LogRequestBody() && r.Method != http.MethodGet && r.Method != http.MethodDelete {
				if len(gcx.RawBody) > 0 {
					logger.AddInfo(ctx, "request", string(gcx.RawBody))
				}
			}

			// Log response body if enabled
			if env.LogResponseBody() && len(rw.body) > 0 {
				logger.AddInfo(ctx, "response", string(rw.body))
			}

			// Log status code
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
