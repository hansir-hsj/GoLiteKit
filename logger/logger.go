package logger

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
)

const (
	LoggerConfigFile = "logger.toml"
	LoggerTextFormat = "text"
	LoggerJSONFormat = "json"
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

type LoggerConfig struct {
	Dir      string `toml:"dir"`
	FileName string `toml:"filename"`
	MinLevel string `toml:"level"`
	Format   string `toml:"format"`

	// Rotator 相关
	MaxAge   time.Duration `toml:"maxAge"`
	MaxSize  int64         `toml:"maxSize"`
	MaxLines int64         `toml:"maxLines"`
}

type Config struct {
	LoggerConfig `toml:"logger"`
}

func parse(conf string) (*Config, error) {
	var lConfig Config
	if err := config.Parse(conf, &lConfig); err != nil {
		return nil, err
	}
	if lConfig.Dir == "" {
		lConfig.Dir = "logs"
	}
	absDir, err := filepath.Abs(lConfig.Dir)
	if err != nil {
		return nil, err
	}
	lConfig.Dir = absDir

	if lConfig.MaxAge <= 30*time.Minute {
		lConfig.MaxAge = 30 * time.Minute
	}
	if lConfig.MaxLines <= 10000 {
		lConfig.MaxLines = 10000
	}
	if lConfig.MaxSize <= 10*1<<20 {
		lConfig.MaxSize = 10 * 1 << 20
	}

	return &lConfig, nil
}

func (c *Config) LogFileName() string {
	if c.FileName == "" {
		c.FileName = "app.log"
	}
	return filepath.Join(c.Dir, c.FileName)
}

func (c *Config) PanicFileName() string {
	return filepath.Join(c.Dir, "panic.log")
}

func NewLogger(loggerConfig ...string) (Logger, error) {
	opts := &slog.HandlerOptions{
		Level:     LevelDebug,
		AddSource: false,
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

	if len(loggerConfig) == 0 {
		return NewConsoleLogger(opts)
	}

	logConf, err := parse(loggerConfig[0])
	if err != nil {
		return nil, err
	}

	logLevel, ok := LevelMap[strings.ToUpper(logConf.MinLevel)]
	if !ok {
		return nil, fmt.Errorf("invalid log level: %s", logConf.MinLevel)
	}
	opts.Level = logLevel

	if logConf.Dir != "" && logConf.FileName != "" {
		return NewTextLogger(logConf, opts)
	}

	return NewConsoleLogger(opts)
}
