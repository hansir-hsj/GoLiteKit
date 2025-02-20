package logger

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"time"
)

type ConsoleLogger struct {
	logger *slog.Logger
}

func (l *ConsoleLogger) Debug(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelDebug, format, args...)
}

func (l *ConsoleLogger) Trace(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelTrace, format, args...)
}

func (l *ConsoleLogger) Info(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelInfo, format, args...)
}

func (l *ConsoleLogger) Warning(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelWarning, format, args...)
}

func (l *ConsoleLogger) Fatal(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelFatal, format, args...)
}

func (l *ConsoleLogger) logit(ctx context.Context, level slog.Level, format string, args ...any) {
	l.log(ctx, slog.Level(level), format, args...)
}

func NewConsoleLogger(ctx context.Context, opts *slog.HandlerOptions) (*ConsoleLogger, error) {
	handler := newContextHandler(os.Stdout, LoggerTextFormat, opts)

	return &ConsoleLogger{
		logger: slog.New(handler),
	}, nil
}

func (l *ConsoleLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !l.logger.Enabled(ctx, level) {
		return
	}
	var pc uintptr
	var pcs [1]uintptr
	// skip [runtime.Callers, this function, this function's caller]
	// NOTE: 这里修改 skip 为 4，*slog.Logger.log 源码中 skip 为 3
	runtime.Callers(4, pcs[:])
	pc = pcs[0]
	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.Add(args...)
	if ctx == nil {
		ctx = context.Background()
	}
	_ = l.logger.Handler().Handle(ctx, r)
}
