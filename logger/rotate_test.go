package logger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestFileLogger(t *testing.T) (*FileLogger, string) {
	t.Helper()
	dir := t.TempDir()
	conf := &Config{LoggerConfig: LoggerConfig{
		Dir:        dir,
		FileName:   "app.log",
		Format:     LoggerTextFormat,
		RotateRule: "1hour",
	}}
	l, err := NewTextLogger(conf, newHandlerOptions())
	if err != nil {
		t.Fatalf("NewTextLogger: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l, dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestFileLoggerRotateWritesToFreshFile(t *testing.T) {
	l, dir := newTestFileLogger(t)
	ctx := context.Background()

	l.Info(ctx, "before-rotate")
	archive := l.NewFilePath()
	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	l.Info(ctx, "after-rotate")

	if got := readFile(t, archive); !strings.Contains(got, "before-rotate") || strings.Contains(got, "after-rotate") {
		t.Fatalf("archive %s content = %q", archive, got)
	}
	if got := readFile(t, filepath.Join(dir, "app.log")); !strings.Contains(got, "after-rotate") {
		t.Fatalf("active log content = %q, want after-rotate", got)
	}
}

func TestFileLoggerRotateDoesNotOverwriteArchive(t *testing.T) {
	l, dir := newTestFileLogger(t)
	ctx := context.Background()

	l.Info(ctx, "first")
	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate 1: %v", err)
	}
	l.Info(ctx, "second")
	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate 2: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var all string
	for _, e := range entries {
		if e.Name() != "app.log" {
			all += readFile(t, filepath.Join(dir, e.Name()))
		}
	}
	if !strings.Contains(all, "first") || !strings.Contains(all, "second") {
		t.Fatalf("archives lost data: %q", all)
	}
}

func TestContextFieldsAttachToHigherLevelRecords(t *testing.T) {
	var buf bytes.Buffer
	h := newContextHandler(&buf, LoggerTextFormat, newHandlerOptions())
	ctx := WithLoggerContext(context.Background())
	AddInfo(ctx, "logid", "abc")
	AddWarning(ctx, "warn_only", "w")

	log := slog.New(h)
	log.ErrorContext(ctx, "error-line")
	log.InfoContext(ctx, "info-line")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "logid=abc") || !strings.Contains(lines[0], "warn_only=w") {
		t.Fatalf("error line missing fields: %q", lines[0])
	}
	if !strings.Contains(lines[1], "logid=abc") || strings.Contains(lines[1], "warn_only") {
		t.Fatalf("info line fields wrong: %q", lines[1])
	}
}

func TestNextRotation(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	at := time.Date(2026, 11, 17, 23, 47, 30, 0, loc)
	for rule, want := range map[string]time.Time{
		"1min":  time.Date(2026, 11, 17, 23, 48, 0, 0, loc),
		"5min":  time.Date(2026, 11, 17, 23, 50, 0, 0, loc),
		"30min": time.Date(2026, 11, 18, 0, 0, 0, 0, loc),
		"1hour": time.Date(2026, 11, 18, 0, 0, 0, 0, loc),
		"1day":  time.Date(2026, 11, 18, 0, 0, 0, 0, loc),
	} {
		if got := nextRotation(at, rule); !got.Equal(want) {
			t.Errorf("%s: next = %v, want %v", rule, got, want)
		}
	}
	if !nextRotation(at, "no").IsZero() || rotationDue(time.Time{}, at) {
		t.Error(`"no" must never rotate`)
	}
}

// When DST ends, local 01:xx occurs twice. Truncating the second 01:30 lands
// on the first 01:00; the next rotation must still be after now, or every
// record would rotate.
func TestNextRotationAfterDSTFallBack(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	now := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC).In(loc) // 01:30 PST
	for _, rule := range []string{"1min", "5min", "10min", "30min", "1hour", "1day"} {
		next := nextRotation(now, rule)
		if !next.After(now) {
			t.Errorf("%s: next %v is not after %v", rule, next, now)
		}
	}
	if got, want := nextRotation(now, "1hour"), time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("1hour: next = %v, want %v", got, want)
	}
}

// After Close, records go to stderr; nothing reopens the log file.
func TestFileLoggerAfterCloseDoesNotReopen(t *testing.T) {
	l, dir := newTestFileLogger(t)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := l.Rotate(); err == nil {
		t.Fatal("Rotate after Close succeeded")
	}
	l.mu.Lock()
	l.nextRotate = time.Now().Add(-time.Second)
	l.mu.Unlock()
	l.Info(context.Background(), "late record")

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		t.Fatal("log file reopened after Close")
	}
	if strings.Contains(readFile(t, filepath.Join(dir, "app.log")), "late record") {
		t.Fatal("record written to the log file after Close")
	}
}

// Records written concurrently with rotations must all land in some file:
// rotation takes the write lock, so it never closes a file mid-write.
func TestFileLoggerConcurrentLoggingAcrossRotations(t *testing.T) {
	l, dir := newTestFileLogger(t)
	ctx := context.Background()

	const writers, perWriter = 8, 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				l.Info(ctx, "line")
			}
		}()
	}
	for i := 0; i < 20; i++ {
		if err := l.Rotate(); err != nil {
			t.Fatalf("Rotate: %v", err)
		}
	}
	wg.Wait()
	l.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, e := range entries {
		lines += strings.Count(readFile(t, filepath.Join(dir, e.Name())), "msg=line")
	}
	if lines != writers*perWriter {
		t.Fatalf("found %d records, want %d", lines, writers*perWriter)
	}
}

// An archive left from an earlier period is rotated at startup, and that
// rotation now also triggers cleanup of excess archives.
func TestStartupRotationCleansOldArchives(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "app.log")
	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 5; i++ {
		p := fmt.Sprintf("%s.2020010%d", active, i+1)
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, old, old)
	}
	if err := os.WriteFile(active, []byte("stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(active, old, old)

	l, err := NewTextLogger(&Config{LoggerConfig: LoggerConfig{
		Dir: dir, FileName: "app.log", Format: LoggerTextFormat, RotateRule: "1day", MaxFileNum: 3,
	}}, newHandlerOptions())
	if err != nil {
		t.Fatal(err)
	}
	l.Close() // waits for the pending cleanup

	entries, _ := os.ReadDir(dir)
	archives := 0
	for _, e := range entries {
		if e.Name() != "app.log" {
			archives++
		}
	}
	if archives != 3 {
		t.Fatalf("archives = %d, want 3 after startup cleanup", archives)
	}
}

func TestFileCleanerMergesKicksAndCloseWaits(t *testing.T) {
	var runs atomic.Int32
	release := make(chan struct{})
	c := newFileCleaner(func() {
		runs.Add(1)
		<-release
	})
	c.kick() // starts a run that blocks
	for runs.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		c.kick() // merged into a single pending run
	}
	close(release)
	c.close()
	if got := runs.Load(); got != 2 {
		t.Fatalf("cleanup runs = %d, want 2 (one running + one merged pending)", got)
	}
}

// BenchmarkFileLoggerFilteredParallel measures records dropped by level,
// which no longer serialize on the logger.
func BenchmarkFileLoggerFilteredParallel(b *testing.B) {
	dir := b.TempDir()
	conf := filepath.Join(dir, "logger.toml")
	content := fmt.Sprintf("[logger]\ndir = %q\nfilename = \"bench.log\"\nlevel = \"info\"\nrotateRule = \"1hour\"\n", dir)
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		b.Fatal(err)
	}
	l, err := NewLogger(conf)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Debug(ctx, "filtered")
		}
	})
}

func BenchmarkFileLoggerParallel(b *testing.B) {
	dir := b.TempDir()
	conf := filepath.Join(dir, "logger.toml")
	content := fmt.Sprintf("[logger]\ndir = %q\nfilename = \"bench.log\"\nlevel = \"info\"\nrotateRule = \"1hour\"\n", dir)
	if err := os.WriteFile(conf, []byte(content), 0644); err != nil {
		b.Fatal(err)
	}
	l, err := NewLogger(conf)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 1 written record per 4 calls; the rest are filtered by level.
			if i%4 == 0 {
				l.Info(ctx, "written")
			} else {
				l.Debug(ctx, "filtered")
			}
			i++
		}
	})
}

func TestLevelOrder(t *testing.T) {
	if !(LevelTrace < LevelDebug && LevelDebug < LevelInfo && LevelInfo < LevelWarning &&
		LevelWarning < LevelError && LevelError < LevelFatal) {
		t.Fatal("levels must be ordered TRACE < DEBUG < INFO < WARN < ERROR < FATAL")
	}

	var buf bytes.Buffer
	opts := newHandlerOptions()
	opts.Level = LevelDebug
	log := slog.New(newContextHandler(&buf, LoggerTextFormat, opts))
	ctx := context.Background()
	log.Log(ctx, LevelTrace, "trace-line")
	log.Log(ctx, LevelDebug, "debug-line")
	if strings.Contains(buf.String(), "trace-line") || !strings.Contains(buf.String(), "debug-line") {
		t.Fatalf("level=debug output = %q, want debug but not trace", buf.String())
	}
}

func TestPanicLoggerReportsFullStackWithPanicSite(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "logger.toml")
	if err := os.WriteFile(conf, []byte("[logger]\ndir = \""+dir+"\"\nrotateRule = \"no\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pl, err := NewPanicLogger(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer pl.Close()

	func() {
		defer func() {
			if p := recover(); p != nil {
				pl.Report(context.Background(), p)
			}
		}()
		deepPanic(300)
	}()

	got := readFile(t, filepath.Join(dir, "panic.log"))
	if !strings.Contains(got, "Recover from panic: boom") {
		t.Fatalf("panic log missing message: %.200q", got)
	}
	// The panic site is in the stack, and a deep stack is not cut at 4KB.
	if !strings.Contains(got, "deepPanic") || len(got) <= 4096 {
		t.Fatalf("stack incomplete (len %d): %.300q", len(got), got)
	}
}

func deepPanic(n int) {
	if n == 0 {
		panic("boom")
	}
	deepPanic(n - 1)
}

func TestUserLevelKeyDoesNotPanic(t *testing.T) {
	var buf bytes.Buffer
	h := newContextHandler(&buf, LoggerTextFormat, newHandlerOptions())
	ctx := WithLoggerContext(context.Background())
	AddInfo(ctx, "level", "custom")

	slog.New(h).InfoContext(ctx, "msg", "level", 3)

	if !strings.Contains(buf.String(), "level=INFO") {
		t.Fatalf("output = %q", buf.String())
	}
}
