package logger

import (
	"context"
	"log/slog"
)

const (
	LevelDebug   = slog.LevelDebug
	LevelTrace   = slog.Level(-2)
	LevelInfo    = slog.LevelInfo
	LevelWarning = slog.LevelWarn
	LevelError   = slog.LevelError
	LevelFatal   = slog.Level(12)
)

type Logger interface {
	Debug(ctx context.Context, format string, args ...any)
	Trace(ctx context.Context, format string, args ...any)
	Info(ctx context.Context, format string, args ...any)
	Warning(ctx context.Context, format string, args ...any)
	Fatal(ctx context.Context, format string, args ...any)
}

var LevelNames = map[slog.Leveler]string{
	LevelTrace: "TRACE",
	LevelFatal: "FATAL",
}
