package logger

import (
	"context"
	"fmt"
	"github/hsj/golite/config"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LoggerConfigFile = "logger.toml"
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

var LevelMap = map[string]slog.Level{
	"TRACE": LevelTrace,
	"DEBUG": LevelDebug,
	"INFO":  LevelInfo,
	"WARN":  LevelWarning,
	"ERROR": LevelError,
	"FATAL": LevelFatal,
}

type LogConfig struct {
	Dir      string `toml:"dir"`
	FileName string `toml:"filename"`
	MinLevel string `toml:"level"`
	Format   string `toml:"format"`

	// Rotator 相关
	MaxAge   time.Duration `toml:"maxAge"`
	MaxSize  int64         `toml:"maxSize"`
	MaxLines int64         `toml:"maxLines"`
}

func parse(conf string) (*LogConfig, error) {
	var logConfig LogConfig
	if err := config.Parse(conf, &logConfig); err != nil {
		return nil, err
	}
	if logConfig.Dir == "" {
		logConfig.Dir = "logs"
	}
	absDir, err := filepath.Abs(logConfig.Dir)
	if err != nil {
		return nil, err
	}
	logConfig.Dir = absDir

	if logConfig.MaxAge <= 30*time.Minute {
		logConfig.MaxAge = 30 * time.Minute
	}
	if logConfig.MaxLines <= 10000 {
		logConfig.MaxLines = 10000
	}
	if logConfig.MaxSize <= 10*1<<20 {
		logConfig.MaxSize = 10 * 1 << 20
	}

	return &logConfig, nil
}

func NewLogger(ctx context.Context, confDir ...string) (Logger, error) {
	opts := &slog.HandlerOptions{
		Level:     LevelDebug,
		AddSource: true,
		// 自定义日志级别
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

	if len(confDir) == 0 {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		return NewConsoleLogger(ctx, &LogConfig{Dir: dir, MinLevel: "debug", Format: "text"}, opts)
	}

	loggerConfig := filepath.Join(confDir[0], LoggerConfigFile)

	logConf, err := parse(loggerConfig)
	if err != nil {
		return nil, err
	}

	logLevel, ok := LevelMap[strings.ToUpper(logConf.MinLevel)]
	if !ok {
		return nil, fmt.Errorf("invalid log level: %s", logConf.MinLevel)
	}
	opts.Level = logLevel

	if logConf.Dir != "" && logConf.FileName != "" {
		return NewTextLogger(ctx, logConf, opts)
	}

	return NewConsoleLogger(ctx, logConf, opts)
}
