package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type PanicLogger struct {
	filePath string
	file     *os.File
}

func NewPanicLogger(ctx context.Context, confDir ...string) (*PanicLogger, error) {
	var filePath string

	if len(confDir) == 0 {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		filePath = filepath.Join(dir, "/log/panic.log")
	} else {
		loggerConfig := filepath.Join(confDir[0], LoggerConfigFile)
		logConf, err := parse(loggerConfig)
		if err != nil {
			return nil, err
		}
		filePath = filepath.Join(logConf.Dir, "panic.log")
	}

	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	return &PanicLogger{
		filePath: filePath,
		file:     target,
	}, nil
}

func (l *PanicLogger) Report(ctx context.Context, p any) {
	msg := fmt.Sprintf("Recover from panic: %v\n", p)
	stack := make([]byte, 0, 4096)
	length := runtime.Stack(stack, false)
	stack = stack[:length]
	fmt.Fprintf(l.file, "%s%s\n", msg, stack)
}
