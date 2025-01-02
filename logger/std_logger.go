package logger

import (
	"context"
	"log/slog"
	"os"
)

type StdLogger struct {
	*slog.Logger
}

func (l *StdLogger) Debug(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelDebug, format, args...)
}

func (l *StdLogger) Trace(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelTrace, format, args...)
}

func (l *StdLogger) Info(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelInfo, format, args...)
}

func (l *StdLogger) Warning(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelWarning, format, args...)
}

func (l *StdLogger) Fatal(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelFatal, format, args...)
}

func (l *StdLogger) log(ctx context.Context, level slog.Level, format string, args ...any) {
	l.Log(ctx, slog.Level(level), format, args...)
}

func NewStdLogger(logConf *LogConfig, opts *slog.HandlerOptions) (*StdLogger, error) {
	var handler slog.Handler
	switch logConf.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		fallthrough
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return &StdLogger{
		slog.New(handler),
	}, nil
}
