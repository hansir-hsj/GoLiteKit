package logger

import (
	"context"
	"testing"
)

func TestLoggerFromConfig(t *testing.T) {
	ctx := WithContext(context.Background())
	log, _ := NewLogger(ctx, "logger.toml")
	log.Debug(ctx, "debug")
	log.Trace(ctx, "trace")
	log.Info(ctx, "info")
	log.Warning(ctx, "warning")
	log.Fatal(ctx, "fatal")

	AddDebug(ctx, "request-id", "request-id_testing")
	AddInfo(ctx, "request-time", "request-time_testing")
	AddWarning(ctx, "request-day", "request-day_testing")
	log.Info(ctx, "info with context")
}
