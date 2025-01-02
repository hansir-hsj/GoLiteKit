package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerFromConfig(t *testing.T) {
	log, _ := NewLogger("logger.toml")
	ctx := context.Background()
	log.Debug(ctx, "debug")
	log.Trace(ctx, "trace")
	log.Info(ctx, "info")
	log.Warning(ctx, "warning")
	log.Fatal(ctx, "fatal")

	ctx = AppendCtx(ctx, slog.String("request_id", "req-123"))
	log.Info(ctx, "info twice")
}
