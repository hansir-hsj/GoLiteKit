package golitekit

import (
	"context"
	"net/http"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func LogIDMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
			ctx = withContext(ctx)
			ctx = logger.WithLoggerContext(ctx)
			logID := EnsureLogID(ctx)
			if logID != "" {
				logger.AddInfo(ctx, "logid", logID)
			}
			return next(ctx, w, r.WithContext(ctx))
		}
	}
}
