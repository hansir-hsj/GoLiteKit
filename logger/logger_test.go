package logger

import (
	"context"
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
}
