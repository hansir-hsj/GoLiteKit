package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var _ Rotator = (*FileLogger)(nil)

type FileLogger struct {
	logConf *Config
	opts    *slog.HandlerOptions

	filePath string

	lastRotate time.Time

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

	if err := rotateExistingFileIfNeeded(filePath, logConf); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to rotate existing log file: %v\n", err)
	}

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
		lastRotate: time.Now(),
	}, nil
}

func rotateExistingFileIfNeeded(filePath string, logConf *Config) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size() == 0 {
		return nil
	}

	modTime := info.ModTime()
	now := time.Now()

	needRotate := false
	switch logConf.RotateRule {
	case "no":
		needRotate = false
	case "1min":
		needRotate = truncateToMinute(modTime) != truncateToMinute(now)
	case "5min":
		needRotate = truncateToMinuteInterval(modTime, 5) != truncateToMinuteInterval(now, 5)
	case "10min":
		needRotate = truncateToMinuteInterval(modTime, 10) != truncateToMinuteInterval(now, 10)
	case "30min":
		needRotate = truncateToMinuteInterval(modTime, 30) != truncateToMinuteInterval(now, 30)
	case "1hour":
		needRotate = truncateToHour(modTime) != truncateToHour(now)
	case "1day":
		needRotate = truncateToDay(modTime) != truncateToDay(now)
	}

	if !needRotate {
		return nil
	}

	var newFilePath string
	switch logConf.RotateRule {
	case "1min":
		newFilePath = filePath + "." + truncateToMinute(modTime).Format("20060102150405")
	case "5min":
		newFilePath = filePath + "." + truncateToMinuteInterval(modTime, 5).Format("20060102150405")
	case "10min":
		newFilePath = filePath + "." + truncateToMinuteInterval(modTime, 10).Format("20060102150405")
	case "30min":
		newFilePath = filePath + "." + truncateToMinuteInterval(modTime, 30).Format("20060102150405")
	case "1hour":
		newFilePath = filePath + "." + truncateToHour(modTime).Format("2006010215")
	case "1day":
		newFilePath = filePath + "." + truncateToDay(modTime).Format("20060102")
	default:
		return nil
	}

	return os.Rename(filePath, newFilePath)
}

func (l *FileLogger) needRotate() bool {
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

func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.needRotate()
}

// rotate opens a new file first, then renames the old one, then swaps handles.
// This ensures l.file is always valid even if rename fails.
func (l *FileLogger) rotate() error {
	newFilePath := l.newFilePath(l.lastRotate)

	// Step 1: Open new file first
	newTarget, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("rotate: open new file failed: %w", err)
	}

	// Step 2: Close and rename old file
	oldTarget := l.file
	if err := oldTarget.Close(); err != nil {
		newTarget.Close()
		return fmt.Errorf("rotate: close failed: %w", err)
	}
	if err := os.Rename(l.filePath, newFilePath); err != nil {
		newTarget.Close()
		l.file, _ = os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		return fmt.Errorf("rotate: rename failed: %w", err)
	}

	// Step 3: Swap handle
	l.file = newTarget
	handler := newContextHandler(newTarget, l.logConf.Format, l.opts)
	l.logger = slog.New(handler)
	l.lastRotate = time.Now()

	go l.cleanOldFiles()

	return nil
}

func (l *FileLogger) cleanOldFiles() {
	if l.logConf.MaxFileNum <= 0 {
		return
	}
	cleanOldLogFiles(l.logConf.Dir, l.filePath, l.logConf.MaxFileNum)
}

func cleanOldLogFiles(dir string, filePath string, maxFileNum int) {
	if maxFileNum <= 0 {
		return
	}

	baseFileName := filepath.Base(filePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read log directory for cleanup: %v\n", err)
		return
	}

	var rotatedFiles []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > len(baseFileName)+1 && name[:len(baseFileName)+1] == baseFileName+"." {
			suffix := name[len(baseFileName)+1:]
			if len(suffix) >= 8 && isDigits(suffix[:8]) {
				rotatedFiles = append(rotatedFiles, entry)
			}
		}
	}

	if len(rotatedFiles) <= maxFileNum {
		return
	}

	sortFilesByModTime(dir, rotatedFiles)

	deleteCount := len(rotatedFiles) - maxFileNum
	for i := 0; i < deleteCount; i++ {
		targetPath := filepath.Join(dir, rotatedFiles[i].Name())
		if err := os.Remove(targetPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to remove old log file %s: %v\n", targetPath, err)
		}
	}
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func sortFilesByModTime(dir string, files []os.DirEntry) {
	type entryWithTime struct {
		entry   os.DirEntry
		modTime time.Time
		valid   bool
	}

	infos := make([]entryWithTime, len(files))
	for i, f := range files {
		info, err := f.Info()
		if err == nil {
			infos[i] = entryWithTime{entry: f, modTime: info.ModTime(), valid: true}
		} else {
			infos[i] = entryWithTime{entry: f, valid: false}
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].valid {
			return false
		}
		if !infos[j].valid {
			return true
		}
		return infos[i].modTime.Before(infos[j].modTime)
	})

	for i, info := range infos {
		files[i] = info.entry
	}
}

func (l *FileLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

func (l *FileLogger) rotateIfNeeded() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.needRotate() {
		return l.rotate()
	}
	return nil
}

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

func (l *FileLogger) NewFilePath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.newFilePath(l.lastRotate)
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
	l.log(ctx, level, format, args...)
}

func (l *FileLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.logger.Enabled(ctx, level) {
		return
	}

	if l.needRotate() {
		if err := l.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	if err := logRecord(ctx, l.logger.Handler(), level, msg, 5, args...); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log message: %v\n", err)
	}
}
