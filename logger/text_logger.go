package logger

import (
	"context"
	"fmt"
	"github/hsj/golite/config"
	"log/slog"
	"os"
	"path/filepath"
)

type TextLogger struct {
	LoggerDir      string     `toml:"dir"`
	LoggerFileName string     `toml:"filename"`
	Level          slog.Level `toml:"level"`

	logger *slog.Logger
}

func NewLogger(confFile string) Logger {
	var defLogger TextLogger
	config.Parse(confFile, &defLogger)
	absDirPath, _ := filepath.Abs(defLogger.LoggerDir)
	fmt.Println("dir", absDirPath)

	err := os.MkdirAll(defLogger.LoggerDir, 0755)
	if err != nil {
		panic(err)
	}
	target, err := os.OpenFile(fmt.Sprintf("%s/%s", defLogger.LoggerDir, defLogger.LoggerFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}

	opts := &slog.HandlerOptions{
		Level: LevelDebug,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.LevelKey {
				level := attr.Value.Any().(slog.Level)
				levelLabel, exists := LevelNames[level]
				if !exists {
					levelLabel = level.String()
				}
				attr.Value = slog.StringValue(levelLabel)
			}
			return attr
		},
	}

	handler := slog.NewTextHandler(target, opts)

	logger := slog.New(handler)
	defLogger.logger = logger

	return &defLogger
}

func (l *TextLogger) Debug(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelDebug, format, args...)
}

func (l *TextLogger) Trace(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelTrace, format, args...)
}

func (l *TextLogger) Info(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelInfo, format, args...)
}

func (l *TextLogger) Warning(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelWarning, format, args...)
}

func (l *TextLogger) Fatal(ctx context.Context, format string, args ...any) {
	l.log(ctx, LevelFatal, format, args...)
}

func (l *TextLogger) log(ctx context.Context, level slog.Level, format string, args ...any) {
	l.logger.Log(ctx, slog.Level(level), format, args...)
}
