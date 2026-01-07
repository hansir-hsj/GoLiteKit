package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
		logConf:    logConf,
		opts:       opts,
		filePath:   filePath,
		logger:     slog.New(handler),
		file:       target,
		LastRotate: time.Now(), // 初始化为当前时间，避免创建后立即轮转
	}, nil
}

// needRotate checks if rotation is needed (internal, no lock)
func (l *FileLogger) needRotate() bool {
	now := time.Now()
	last := l.LastRotate

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

// NeedRotate checks if rotation is needed (thread-safe)
func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.needRotate()
}

// rotate performs the actual rotation (internal, no lock)
func (l *FileLogger) rotate() error {
	// 使用上次轮转时间生成文件名，确保文件名对应正确的时间周期
	newFilePath := l.newFilePath(l.LastRotate)

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
	handler := newContextHandler(newTarget, l.logConf.Format, l.opts)
	l.logger = slog.New(handler)

	l.lines = 0
	l.LastRotate = time.Now()

	return nil
}

// Rotate performs rotation (thread-safe)
func (l *FileLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

// rotateIfNeeded atomically checks and performs rotation if needed
func (l *FileLogger) rotateIfNeeded() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.needRotate() {
		return l.rotate()
	}
	return nil
}

// newFilePath generates new file path based on the given time (internal)
func (l *FileLogger) newFilePath(t time.Time) string {
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

// NewFilePath generates new file path (for Rotator interface compatibility)
func (l *FileLogger) NewFilePath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.newFilePath(l.LastRotate)
}

func (l *FileLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelDebug, msg, args...)
}

func (l *FileLogger) Trace(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelTrace, msg, args...)
}

func (l *FileLogger) Info(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelInfo, msg, args...)
}

func (l *FileLogger) Warning(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelWarning, msg, args...)
}

func (l *FileLogger) Error(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelError, msg, args...)
}

func (l *FileLogger) Fatal(ctx context.Context, msg string, args ...any) {
	l.logit(ctx, LevelFatal, msg, args...)
}

func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *FileLogger) logit(ctx context.Context, level slog.Level, format string, args ...any) {
	l.log(ctx, slog.Level(level), format, args...)
}

func (l *FileLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !l.logger.Enabled(ctx, level) {
		return
	}

	// 原子操作：检查并轮转
	if err := l.rotateIfNeeded(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
	}

	// callerSkip=5: logRecord -> log -> logit -> Debug/Info/... -> user code
	if err := logRecord(ctx, l.logger.Handler(), level, msg, 5, args...); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log message: %v\n", err)
		return
	}

	l.lines++
}
