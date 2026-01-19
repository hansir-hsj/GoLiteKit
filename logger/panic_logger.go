package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PanicLogger struct {
	logConf    *Config
	filePath   string
	file       *os.File
	lastRotate time.Time
	mu         sync.Mutex
}

func NewPanicLogger(loggerConfig ...string) (*PanicLogger, error) {
	var filePath string
	var logConf *Config

	if len(loggerConfig) == 0 {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		filePath = filepath.Join(dir, "/log/panic.log")
		logConf = &Config{
			LoggerConfig: LoggerConfig{
				RotateRule: "1day",
				MaxFileNum: 30,
			},
		}
	} else {
		var err error
		logConf, err = parse(loggerConfig[0])
		if err != nil {
			return nil, err
		}
		filePath = logConf.PanicFileName()
	}

	// Check and rotate existing file if needed
	if err := rotateExistingFileIfNeeded(filePath, logConf); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to rotate existing panic log file: %v\n", err)
	}

	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	return &PanicLogger{
		logConf:    logConf,
		filePath:   filePath,
		file:       target,
		lastRotate: time.Now(),
	}, nil
}

func (l *PanicLogger) caller() string {
	_, file, line, ok := runtime.Caller(4)
	if !ok {
		return ""
	}
	return strings.Join([]string{file, strconv.Itoa(line)}, ":")
}

// needRotate checks if rotation is needed (internal, no lock)
func (l *PanicLogger) needRotate() bool {
	if l.logConf == nil {
		return false
	}

	now := time.Now()
	last := l.lastRotate

	switch l.logConf.RotateRule {
	case "no":
		return false
	case "1min":
		return truncateToMinute(last) != truncateToMinute(now)
	case "5min":
		return truncateToMinuteInterval(last, 5) != truncateToMinuteInterval(now, 5)
	case "10min":
		return truncateToMinuteInterval(last, 10) != truncateToMinuteInterval(now, 10)
	case "30min":
		return truncateToMinuteInterval(last, 30) != truncateToMinuteInterval(now, 30)
	case "1hour":
		return truncateToHour(last) != truncateToHour(now)
	case "1day":
		return truncateToDay(last) != truncateToDay(now)
	}

	return false
}

// rotate performs the actual rotation (internal, no lock)
func (l *PanicLogger) rotate() error {
	newFilePath := l.newFilePath(l.lastRotate)

	if err := l.file.Close(); err != nil {
		return err
	}
	if err := os.Rename(l.filePath, newFilePath); err != nil {
		return err
	}
	newTarget, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.file = newTarget
	l.lastRotate = time.Now()

	return nil
}

// rotateIfNeeded atomically checks and performs rotation if needed
func (l *PanicLogger) rotateIfNeeded() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.needRotate() {
		return l.rotate()
	}
	return nil
}

// newFilePath generates new file path based on the given time (internal)
func (l *PanicLogger) newFilePath(t time.Time) string {
	if l.logConf == nil {
		return l.filePath
	}

	switch l.logConf.RotateRule {
	case "no":
		return l.filePath
	case "1min":
		return l.filePath + "." + truncateToMinute(t).Format("20060102150405")
	case "5min":
		return l.filePath + "." + truncateToMinuteInterval(t, 5).Format("20060102150405")
	case "10min":
		return l.filePath + "." + truncateToMinuteInterval(t, 10).Format("20060102150405")
	case "30min":
		return l.filePath + "." + truncateToMinuteInterval(t, 30).Format("20060102150405")
	case "1hour":
		return l.filePath + "." + truncateToHour(t).Format("2006010215")
	case "1day":
		return l.filePath + "." + truncateToDay(t).Format("20060102")
	}

	return l.filePath
}

func (l *PanicLogger) Report(ctx context.Context, p any) {
	// Check and rotate if needed
	if err := l.rotateIfNeeded(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to rotate panic log: %v\n", err)
	}

	msg := fmt.Sprintf("[%s] Recover from panic: %v", time.Now().Format("2006-01-02 15:04:05.000"), p)
	stack := make([]byte, 4096)
	length := runtime.Stack(stack, false)
	stack = stack[:length]

	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := fmt.Fprintf(l.file, "%s\n%s\nStack:\n%s\n\n", msg, l.caller(), stack); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write panic log: %v\n", err)
	}
}

func (l *PanicLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
