package logger

import (
	"context"
	"log/slog"
	"os"
)

type ConsoleLogger struct {
	logger *slog.Logger
}

func (l *ConsoleLogger) Debug(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelDebug, format, args...)
}

func (l *ConsoleLogger) Trace(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelTrace, format, args...)
}

func (l *ConsoleLogger) Info(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelInfo, format, args...)
}

func (l *ConsoleLogger) Warning(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelWarning, format, args...)
}

func (l *ConsoleLogger) Fatal(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelFatal, format, args...)
}

func (l *ConsoleLogger) log(ctx context.Context, level slog.Level, format string, args ...any) {
	l.logger.Log(ctx, slog.Level(level), format, args...)
}

func NewConsoleLogger(ctx context.Context, logConf *LogConfig, opts *slog.HandlerOptions) (*ConsoleLogger, error) {
	handler := newContextHandler(os.Stdout, logConf.Format, opts)

	return &ConsoleLogger{
		logger: slog.New(handler),
	}, nil
}
