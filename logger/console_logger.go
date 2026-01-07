package logger

import (
	"context"
	"log/slog"
	"os"
)

type ConsoleLogger struct {
	logger *slog.Logger
}

func (l *ConsoleLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelDebug, msg, args...)
}

func (l *ConsoleLogger) Trace(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelTrace, msg, args...)
}

func (l *ConsoleLogger) Info(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelInfo, msg, args...)
}

func (l *ConsoleLogger) Warning(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelWarning, msg, args...)
}

func (l *ConsoleLogger) Error(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelError, msg, args...)
}

func (l *ConsoleLogger) Fatal(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelFatal, msg, args...)
}

func (l *ConsoleLogger) Close() error {
	// ConsoleLogger 输出到 stdout，无需关闭
	return nil
}

func (l *ConsoleLogger) logit(ctx context.Context, level slog.Level, format string, args ...any) {
	l.log(ctx, slog.Level(level), format, args...)
}

func NewConsoleLogger(opts *slog.HandlerOptions) (*ConsoleLogger, error) {
	handler := newContextHandler(os.Stdout, LoggerTextFormat, opts)

	return &ConsoleLogger{
		logger: slog.New(handler),
	}, nil
}

func (l *ConsoleLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !l.logger.Enabled(ctx, level) {
		return
	}
	// callerSkip=5: logRecord -> log -> logit -> Debug/Info/... -> user code
	_ = logRecord(ctx, l.logger.Handler(), level, msg, 5, args...)
}
