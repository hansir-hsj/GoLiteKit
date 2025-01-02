package logger

import (
	"fmt"
	"log/slog"
	"os"
)

type TextLogger struct {
	StdLogger
}

func NewTextLogger(logConf *LogConfig, opts *slog.HandlerOptions) (*TextLogger, error) {
	err := os.MkdirAll(logConf.Dir, 0755)
	if err != nil {
		return nil, err
	}
	target, err := os.OpenFile(fmt.Sprintf("%s/%s", logConf.Dir, logConf.FileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	switch logConf.Format {
	case "json", "JSON":
		handler = slog.NewJSONHandler(target, opts)
	case "text", "TEXT":
		fallthrough
	default:
		handler = slog.NewTextHandler(target, opts)
	}

	return &TextLogger{
		StdLogger{slog.New(handler)},
	}, nil
}
