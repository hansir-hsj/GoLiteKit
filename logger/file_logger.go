package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type TextLogger struct {
	ConsoleLogger
}

func NewTextLogger(ctx context.Context, logConf *LogConfig, opts *slog.HandlerOptions) (*TextLogger, error) {
	err := os.MkdirAll(logConf.Dir, 0755)
	if err != nil {
		return nil, err
	}
	target, err := os.OpenFile(fmt.Sprintf("%s/%s", logConf.Dir, logConf.FileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	handler := newContextHandler(target, logConf.Format, opts)

	return &TextLogger{
		ConsoleLogger{
			logger: slog.New(handler),
		},
	}, nil
}
