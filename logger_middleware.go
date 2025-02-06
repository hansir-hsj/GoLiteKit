package golite

import (
	"context"
	"github/hsj/golite/env"
	"github/hsj/golite/logger"
	"net/http"
)

func LoggerMiddleware(ctx context.Context, w http.ResponseWriter, req *http.Request, queue MiddlewareQueue) error {
	logInst, err := logger.NewLogger(ctx, env.ConfDir())
	if err != nil {
		return err
	}
	panicLogInst, err := logger.NewPanicLogger(ctx, env.ConfDir())
	if err != nil {
		return err
	}
	gcx := GetContext(ctx)
	gcx.SetContextOptions(WithLogger(logInst), WithPanicLogger(panicLogInst))
	logger.AddInfo(ctx, "method", gcx.request.Method)
	logger.AddInfo(ctx, "url", gcx.request.URL)

	err = queue.Next(ctx, w, req)
	if err != nil {
		logInst.Warning(ctx, err.Error())
		return err
	}
	logInst.Info(ctx, "ok")

	return nil
}
