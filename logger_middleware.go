package golitekit

import (
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/env"
	"github.com/hansir-hsj/GoLiteKit/logger"
)

// responseCapture wraps http.ResponseWriter to capture response body
type responseCapture struct {
	http.ResponseWriter
	body       []byte
	statusCode int
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithContext(r.Context())
			gcx := GetContext(ctx)

			// Wrap ResponseWriter to capture response body
			rw := &responseCapture{ResponseWriter: w, statusCode: http.StatusOK}
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

			logInst.Info(ctx, "ok")
		})
	}
}
