package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var _ Rotator = (*FileLogger)(nil)

var errLoggerClosed = errors.New("logger: closed")

// FileLogger writes records to a file and rotates it by time. mu guards both
// writes and rotation, so a file is never closed mid-write.
type FileLogger struct {
	logConf   *Config
	opts      *slog.HandlerOptions
	addSource bool

	filePath string

	mu         sync.Mutex
	lastRotate time.Time
	nextRotate time.Time // zero: never
	logger     *slog.Logger
	file       *os.File
	closed     bool // after Close, records go to stderr and nothing rotates

	cleaner   *fileCleaner
	closeOnce sync.Once
}

func NewTextLogger(logConf *Config, opts *slog.HandlerOptions) (*FileLogger, error) {
	err := os.MkdirAll(logConf.Dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logConf.Dir, err)
	}

	filePath := logConf.LogFileName()

	rotated, err := rotateExistingFileIfNeeded(filePath, logConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to rotate existing log file: %v\n", err)
	}

	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	handler := newContextHandler(target, logConf.Format, opts)

	now := time.Now()
	l := &FileLogger{
		logConf:    logConf,
		opts:       opts,
		addSource:  opts != nil && opts.AddSource,
		filePath:   filePath,
		logger:     slog.New(handler),
		file:       target,
		lastRotate: now,
		nextRotate: nextRotation(now, logConf.RotateRule),
	}
	if logConf.MaxFileNum > 0 {
		l.cleaner = newFileCleaner(l.cleanOldFiles)
	}
	if rotated {
		l.cleaner.kick()
	}
	return l, nil
}

// rotateExistingFileIfNeeded archives a non-empty log file left from an
// earlier rotation period and reports whether it did.
func rotateExistingFileIfNeeded(filePath string, logConf *Config) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if info.Size() == 0 {
		return false, nil
	}

	modTime := info.ModTime()
	if !rotationDue(nextRotation(modTime, logConf.RotateRule), time.Now()) {
		return false, nil
	}

	newFilePath := archivePath(filePath, logConf.RotateRule, modTime)
	if newFilePath == filePath {
		return false, nil
	}
	if err := os.Rename(filePath, uniqueArchivePath(newFilePath)); err != nil {
		return false, err
	}
	return true, nil
}

// archivePath names the archive for the rotation period containing t.
func archivePath(filePath, rule string, t time.Time) string {
	switch rule {
	case "1min":
		return filePath + "." + truncateToMinute(t).Format("20060102150405")
	case "5min":
		return filePath + "." + truncateToMinuteInterval(t, 5).Format("20060102150405")
	case "10min":
		return filePath + "." + truncateToMinuteInterval(t, 10).Format("20060102150405")
	case "30min":
		return filePath + "." + truncateToMinuteInterval(t, 30).Format("20060102150405")
	case "1hour":
		return filePath + "." + truncateToHour(t).Format("2006010215")
	case "1day":
		return filePath + "." + truncateToDay(t).Format("20060102")
	}
	return filePath
}

// uniqueArchivePath returns path, or path.N when path already exists, so an
// existing archive is never overwritten.
func uniqueArchivePath(path string) string {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path
	}
	for i := 1; ; i++ {
		p := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
	}
}

// rotateFile closes current, renames filePath to archivePath and reopens
// filePath. It returns the new file whenever reopening succeeds.
func rotateFile(current *os.File, filePath, archivePath string) (*os.File, error) {
	var errs []error
	if current != nil {
		if err := current.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close: %w", err))
		}
	}
	if archivePath != filePath {
		if err := os.Rename(filePath, uniqueArchivePath(archivePath)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("rename: %w", err))
		}
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		errs = append(errs, fmt.Errorf("open: %w", err))
		return nil, errors.Join(errs...)
	}
	return f, errors.Join(errs...)
}

func (l *FileLogger) NeedRotate() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return rotationDue(l.nextRotate, time.Now())
}

// rotate archives the current file and switches the handler to a fresh one.
// The handler is always rebuilt, falling back to stderr if the log file
// cannot be reopened, so it never points at a closed file. Callers hold mu.
func (l *FileLogger) rotate() error {
	f, err := rotateFile(l.file, l.filePath, l.newFilePath(l.lastRotate))

	// Advance even on failure so a persistent error is not retried per record.
	now := time.Now()
	l.lastRotate = now
	l.nextRotate = nextRotation(now, l.logConf.RotateRule)
	l.file = f
	var target io.Writer = os.Stderr
	if f != nil {
		target = f
	}
	l.logger = slog.New(newContextHandler(target, l.logConf.Format, l.opts))

	if err != nil {
		return fmt.Errorf("rotate: %w", err)
	}

	l.cleaner.kick()

	return nil
}

func (l *FileLogger) cleanOldFiles() {
	if l.logConf.MaxFileNum <= 0 {
		return
	}
	// Archives sit next to the log file, which may be in a subdirectory of Dir.
	cleanOldLogFiles(filepath.Dir(l.filePath), l.filePath, l.logConf.MaxFileNum)
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
	if l.closed {
		return errLoggerClosed
	}
	return l.rotate()
}

func (l *FileLogger) newFilePath(t time.Time) string {
	return archivePath(l.filePath, l.logConf.RotateRule, t)
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

// Close closes the log file and waits for a pending cleanup to finish. Only
// the first call does anything. Records logged afterwards go to stderr.
func (l *FileLogger) Close() error {
	var err error
	l.closeOnce.Do(func() {
		l.mu.Lock()
		if l.file != nil {
			err = l.file.Close()
		}
		l.file = nil
		l.closed = true
		l.logger = slog.New(newContextHandler(os.Stderr, l.logConf.Format, l.opts))
		l.mu.Unlock()
		l.cleaner.close()
	})
	return err
}

func (l *FileLogger) logit(ctx context.Context, level slog.Level, format string, args ...any) {
	l.log(ctx, level, format, args...)
}

func (l *FileLogger) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !levelEnabled(l.opts, level) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.closed && rotationDue(l.nextRotate, time.Now()) {
		if err := l.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to rotate log file: %v\n", err)
		}
	}

	// callerSkip=5: logRecord -> log -> logit -> Debug/Info/... -> user code
	if err := logRecord(ctx, l.logger.Handler(), level, msg, 5, l.addSource, args...); err != nil {
		fmt.Fprintf(os.Stderr, "failed to log message: %v\n", err)
	}
}

// levelEnabled mirrors slog's built-in handlers: records below opts.Level
// (INFO if unset) are dropped. It needs no lock; a LevelVar is read atomically.
func levelEnabled(opts *slog.HandlerOptions, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		minLevel = opts.Level.Level()
	}
	return level >= minLevel
}
