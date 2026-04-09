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

	// Rotate existing file if needed before opening
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
		LastRotate: time.Now(),
	}, nil
}

// rotateExistingFileIfNeeded rotates old log files from previous time periods.
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

// needRotate checks if rotation is needed.
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

// NeedRotate checks if rotation is needed (thread-safe).
func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.needRotate()
}

// rotate performs the actual rotation.
// If any step fails the logger always ends up with a valid open file handle
// so that subsequent log calls do not write to a closed fd.
func (l *FileLogger) rotate() error {
	newFilePath := l.newFilePath(l.LastRotate)

	if err := l.file.Close(); err != nil {
		// Could not close the current file. Reopen it so the handle stays valid.
		f, openErr := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if openErr == nil {
			l.file = f
			handler := newContextHandler(f, l.logConf.Format, l.opts)
			l.logger = slog.New(handler)
		}
		return fmt.Errorf("rotate: close failed: %w", err)
	}

	if err := os.Rename(l.filePath, newFilePath); err != nil {
		// Rename failed; reopen the original file so we can keep logging.
		f, openErr := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if openErr == nil {
			l.file = f
			handler := newContextHandler(f, l.logConf.Format, l.opts)
			l.logger = slog.New(handler)
		}
		return fmt.Errorf("rotate: rename failed: %w", err)
	}

	newTarget, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// New file could not be created; try to reopen the renamed file to
		// preserve any ability to log rather than crash.
		f, openErr := os.OpenFile(newFilePath, os.O_APPEND|os.O_WRONLY, 0644)
		if openErr == nil {
			l.file = f
			handler := newContextHandler(f, l.logConf.Format, l.opts)
			l.logger = slog.New(handler)
		}
		return fmt.Errorf("rotate: open new file failed: %w", err)
	}

	l.file = newTarget
	handler := newContextHandler(newTarget, l.logConf.Format, l.opts)
	l.logger = slog.New(handler)

	l.lines = 0
	l.LastRotate = time.Now()

	go l.cleanOldFiles()

	return nil
}

// cleanOldFiles removes old rotated log files.
func (l *FileLogger) cleanOldFiles() {
	if l.logConf.MaxFileNum <= 0 {
		return
	}
	cleanOldLogFiles(l.logConf.Dir, l.filePath, l.logConf.MaxFileNum)
}

// cleanOldLogFiles removes old rotated log files exceeding maxFileNum.
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

// isDigits checks if a string contains only digits.
func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// sortFilesByModTime sorts files by modification time (oldest first).
// It pre-fetches all Info() results in a single pass to avoid the O(n²)
// syscall cost of calling Info() inside a nested comparison loop.
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

// Rotate performs rotation (thread-safe).
func (l *FileLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

// rotateIfNeeded checks and performs rotation if needed.
func (l *FileLogger) rotateIfNeeded() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.needRotate() {
		return l.rotate()
	}
	return nil
}

// newFilePath generates new file path based on the given time.
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

// NewFilePath generates new file path for Rotator interface.
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
		return
	}

	// l.mu is already held exclusively here; a plain increment is sufficient.
	l.lines++
}
