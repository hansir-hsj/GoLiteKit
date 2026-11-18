package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

type PanicLogger struct {
	logConf    *Config
	filePath   string
	file       *os.File
	lastRotate time.Time
	nextRotate time.Time // zero: never
	closed     bool      // after Close, reports go to stderr and nothing rotates
	mu         sync.Mutex

	cleaner   *fileCleaner
	closeOnce sync.Once
}

// NewPanicLogger creates a panic logger from the given config file. With no
// argument or an empty path it writes to ./log/panic.log with daily rotation.
func NewPanicLogger(loggerConfig ...string) (*PanicLogger, error) {
	var filePath string
	var logConf *Config

	if len(loggerConfig) == 0 || loggerConfig[0] == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		filePath = filepath.Join(dir, "log", "panic.log")
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

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create panic log directory: %w", err)
	}

	rotated, err := rotateExistingFileIfNeeded(filePath, logConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to rotate existing panic log file: %v\n", err)
	}

	target, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	l := &PanicLogger{
		logConf:    logConf,
		filePath:   filePath,
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

// rotate archives the current file and reopens a fresh one. l.file is nil
// if reopening failed; Report then falls back to stderr.
func (l *PanicLogger) rotate() error {
	f, err := rotateFile(l.file, l.filePath, l.newFilePath(l.lastRotate))

	// Advance even on failure so a persistent error is not retried per report.
	now := time.Now()
	l.lastRotate = now
	l.nextRotate = nextRotation(now, l.logConf.RotateRule)
	l.file = f

	if err != nil {
		return fmt.Errorf("rotate: %w", err)
	}

	l.cleaner.kick()

	return nil
}

func (l *PanicLogger) cleanOldFiles() {
	dir := filepath.Dir(l.filePath)
	cleanOldLogFiles(dir, l.filePath, l.logConf.MaxFileNum)
}

func (l *PanicLogger) newFilePath(t time.Time) string {
	return archivePath(l.filePath, l.logConf.RotateRule, t)
}

// Report writes p with the current goroutine's stack. Call it from the
// deferred function that recovered the panic.
func (l *PanicLogger) Report(ctx context.Context, p any) {
	// Capture before taking the lock; debug.Stack grows its buffer as needed,
	// so deep stacks are not truncated.
	stack := debug.Stack()

	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.closed && rotationDue(l.nextRotate, time.Now()) {
		if err := l.rotate(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to rotate panic log: %v\n", err)
		}
	}

	msg := fmt.Sprintf("[%s] Recover from panic: %v", time.Now().Format("2006-01-02 15:04:05.000"), p)

	var w io.Writer = os.Stderr
	if l.file != nil {
		w = l.file
	}
	if _, err := fmt.Fprintf(w, "%s\nStack:\n%s\n\n", msg, stack); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write panic log: %v\n", err)
	}
}

// Close closes the panic log file and waits for a pending cleanup to finish.
// Only the first call does anything. Reports written afterwards go to stderr.
func (l *PanicLogger) Close() error {
	var err error
	l.closeOnce.Do(func() {
		l.mu.Lock()
		if l.file != nil {
			err = l.file.Close()
		}
		l.file = nil
		l.closed = true
		l.mu.Unlock()
		l.cleaner.close()
	})
	return err
}
