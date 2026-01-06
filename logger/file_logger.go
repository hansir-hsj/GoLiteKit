package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"
)

var _ Rotator = (*FileLogger)(nil)

type FileLogger struct {
	logConf *Config
	opts    *slog.HandlerOptions

	filePath string

	lines      int64
	LastRotate time.Time

	logger *slog.Logger

	file *os.File

	mu sync.Mutex
}

func NewTextLogger(logConf *Config, opts *slog.HandlerOptions) (*FileLogger, error) {
	err := os.MkdirAll(logConf.Dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logConf.Dir, err)
	}

	filePath := logConf.LogFileName()
	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	handler := newContextHandler(target, logConf.Format, opts)

	return &FileLogger{
		logConf:  logConf,
		opts:     opts,
		filePath: filePath,
		logger:   slog.New(handler),
		file:     target,
	}, nil
}

func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	last := l.LastRotate

	if last.IsZero() {
		return true
	}

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

func (l *FileLogger) Rotate() error {
	if err := l.file.Close(); err != nil {
		return err
	}
	newFilePath := l.NewFilePath(l.filePath)
	if err := os.Rename(l.filePath, newFilePath); err != nil {
		return err
	}
	newTarget, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.file = newTarget
	handler := newContextHandler(newTarget, l.logConf.Format, l.opts)
	l.logger = slog.New(handler)

	l.lines = 0
	l.LastRotate = time.Now()

	return nil
}

func (l *FileLogger) NewFilePath(filePath string) string {
	now := time.Now()

	switch l.logConf.RotateRule {
	case "no":
		return filePath
	case "1min":
		return filePath + "." + truncateToMinute(now).Format("20060102150405")
	case "5min":
		return filePath + "." + truncateToMinuteInterval(now, 5).Format("20060102150405")
	case "10min":
		return filePath + "." + truncateToMinuteInterval(now, 10).Format("20060102150405")
	case "30min":
		return filePath + "." + truncateToMinuteInterval(now, 30).Format("20060102150405")
	case "1hour":
		return filePath + "." + truncateToHour(now).Format("2006010215")
	case "1day":
		return filePath + "." + truncateToDay(now).Format("20060102")
	}

	return filePath
}

func (l *FileLogger) Debug(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelDebug, format, args...)
}

func (l *FileLogger) Trace(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelTrace, format, args...)
}

func (l *FileLogger) Info(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelInfo, format, args...)
}

func (l *FileLogger) Warning(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelWarning, format, args...)
}

func (l *FileLogger) Fatal(ctx context.Context, format string, args ...any) {
	l.logit(ctx, LevelFatal, format, args...)
}

func (l *FileLogger) logit(ctx context.Context, level slog.Level, format string, args ...any) {
	l.log(ctx, slog.Level(level), format, args...)
}

func (l *FileLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !l.logger.Enabled(ctx, level) {
		return
	}

	if l.NeedRotate() {
		l.Rotate()
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
	if err := l.logger.Handler().Handle(ctx, r); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log message: %v\n", err)
		return
	}

	l.lines++
}
