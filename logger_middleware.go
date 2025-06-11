package golitekit

import (
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger) HandlerMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithContext(r.Context())
			gcx := GetContext(ctx)
			gcx.request = r
			gcx.responseWriter = w

			gcx.SetContextOptions(WithLogger(logInst), WithPanicLogger(panicInst))
			logger.AddInfo(ctx, "method", gcx.request.Method)
			logger.AddInfo(ctx, "url", gcx.request.URL)

			next.ServeHTTP(w, r)

			logInst.Info(ctx, "ok")
		})
	}
}
