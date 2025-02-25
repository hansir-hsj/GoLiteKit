package golitekit

import (
	"context"

	"github.com/hansir-hsj/GoLiteKit/logger"
)

func LoggerAsMiddleware(logInst logger.Logger, panicInst *logger.PanicLogger) Middleware {
	return func(ctx context.Context, queue MiddlewareQueue) error {
		gcx := GetContext(ctx)
		gcx.SetContextOptions(WithLogger(logInst), WithPanicLogger(panicInst))
		logger.AddInfo(ctx, "method", gcx.request.Method)
		logger.AddInfo(ctx, "url", gcx.request.URL)

		err := queue.Next(ctx)
		if err != nil {
			logInst.Warning(ctx, err.Error())
			return err
		}
		logInst.Info(ctx, "ok")

		return nil
	}
}
