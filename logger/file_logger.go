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

var _ Rotater = (*FileLogger)(nil)

type FileLogger struct {
	logConf *LogConfig
	opts    *slog.HandlerOptions

	filePath string

	lines      int64
	LastRotate time.Time

	logger *slog.Logger

	file *os.File

	mu sync.Mutex
}

func NewTextLogger(ctx context.Context, logConf *LogConfig, opts *slog.HandlerOptions) (*FileLogger, error) {
	err := os.MkdirAll(logConf.Dir, 0755)
	if err != nil {
		return nil, err
	}
	filePath := fmt.Sprintf("%s/%s", logConf.Dir, logConf.FileName)
	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	handler := newContextHandler(target, logConf.Format, opts)

	return &FileLogger{
		logConf:    logConf,
		opts:       opts,
		filePath:   filePath,
		logger:     slog.New(handler),
		file:       target,
		LastRotate: time.Now(),
	}, nil
}

func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lines >= l.logConf.MaxLines {
		return true
	}

	fi, err := l.file.Stat()
	if err != nil {
		return false
	}
	if fi.Size() >= l.logConf.MaxSize {
		return true
	}
	if time.Since(l.LastRotate) >= l.logConf.MaxAge {
		return true
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
	since := time.Since(l.LastRotate)
	now := time.Now()
	if since > 24*time.Hour {
		return filePath + "." + now.Format("20060102")
	}
	if since > time.Hour {
		return filePath + "." + now.Format("20060102-15")
	}
	if since > 10*time.Minute {
		minute := now.Minute()
		minuteMod := minute % 10
		return filePath + "." + fmt.Sprintf("%s-%02d", now.Format("20060102-15"), minuteMod)
	}

	return filePath + "." + time.Now().Format("20060102-150405")
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
	_ = l.logger.Handler().Handle(ctx, r)

	l.lines++
}
