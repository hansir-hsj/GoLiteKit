package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsDigits(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"12345678", true},
		{"20260120", true},
		{"0000", true},
		{"", true}, // empty string has no non-digit characters
		{"123abc", false},
		{"abc123", false},
		{"12-34", false},
		{"12.34", false},
		{"12 34", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isDigits(tt.input)
			if result != tt.expected {
				t.Errorf("isDigits(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSortFilesByModTime(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_sort_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create files with different modification times
	files := []string{"file1.log", "file2.log", "file3.log"}
	for i, name := range files {
		filePath := filepath.Join(tempDir, name)
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		f.Close()

		// Set modification time (oldest first)
		modTime := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Read directory entries
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	// Sort by modification time (oldest first)
	sortFilesByModTime(tempDir, entries)

	// Verify order (oldest should be first)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// file1 should be first (oldest), file3 should be last (newest)
	if entries[0].Name() != "file1.log" {
		t.Errorf("Expected file1.log first, got %s", entries[0].Name())
	}
	if entries[2].Name() != "file3.log" {
		t.Errorf("Expected file3.log last, got %s", entries[2].Name())
	}
}

func TestCleanOldLogFiles(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")

	// Create current log file
	f, err := os.Create(baseLogFile)
	if err != nil {
		t.Fatalf("Failed to create base log file: %v", err)
	}
	f.Close()

	// Create 5 rotated log files with different timestamps
	rotatedFiles := []string{
		"app.log.20260115",
		"app.log.20260116",
		"app.log.20260117",
		"app.log.20260118",
		"app.log.20260119",
	}

	for i, name := range rotatedFiles {
		filePath := filepath.Join(tempDir, name)
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		f.Close()

		// Set modification time (oldest first)
		modTime := time.Now().Add(-time.Duration(len(rotatedFiles)-i) * 24 * time.Hour)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Verify we have 6 files (1 current + 5 rotated)
	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 6 {
		t.Fatalf("Expected 6 files before cleanup, got %d", len(entries))
	}

	// Clean up, keep only 3 rotated files
	cleanOldLogFiles(tempDir, baseLogFile, 3)

	// Verify we have 4 files now (1 current + 3 rotated)
	entries, _ = os.ReadDir(tempDir)
	if len(entries) != 4 {
		t.Errorf("Expected 4 files after cleanup, got %d", len(entries))
	}

	// Verify current log file still exists
	if _, err := os.Stat(baseLogFile); os.IsNotExist(err) {
		t.Error("Current log file should not be deleted")
	}

	// Verify oldest files were deleted
	for _, name := range []string{"app.log.20260115", "app.log.20260116"} {
		filePath := filepath.Join(tempDir, name)
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("Old file %s should have been deleted", name)
		}
	}

	// Verify newest files still exist
	for _, name := range []string{"app.log.20260117", "app.log.20260118", "app.log.20260119"} {
		filePath := filepath.Join(tempDir, name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("New file %s should still exist", name)
		}
	}
}

func TestCleanOldLogFiles_NoCleanupNeeded(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")

	// Create current log file
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create only 2 rotated files
	for _, name := range []string{"app.log.20260118", "app.log.20260119"} {
		f, _ := os.Create(filepath.Join(tempDir, name))
		f.Close()
	}

	// Clean up with maxFileNum=5 (no cleanup needed)
	cleanOldLogFiles(tempDir, baseLogFile, 5)

	// All files should still exist
	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 3 {
		t.Errorf("Expected 3 files (no cleanup needed), got %d", len(entries))
	}
}

func TestCleanOldLogFiles_ZeroMaxFileNum(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create rotated file
	f, _ = os.Create(filepath.Join(tempDir, "app.log.20260119"))
	f.Close()

	// Clean up with maxFileNum=0 (should do nothing)
	cleanOldLogFiles(tempDir, baseLogFile, 0)

	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 2 {
		t.Errorf("Expected 2 files (maxFileNum=0 should skip cleanup), got %d", len(entries))
	}
}

func TestCleanOldLogFiles_IgnoresNonMatchingFiles(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")

	// Create current log file
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create files that should NOT be matched/deleted
	nonMatchingFiles := []string{
		"other.log",           // different base name
		"app.log.backup",      // non-timestamp suffix
		"app.log.txt",         // non-timestamp suffix
		"app.log.2026abc",     // partial timestamp
		"app.log.",            // empty suffix
		"README.md",           // completely different
	}

	for _, name := range nonMatchingFiles {
		f, _ := os.Create(filepath.Join(tempDir, name))
		f.Close()
	}

	// Create 3 matching rotated files
	matchingFiles := []string{
		"app.log.20260115",
		"app.log.20260116",
		"app.log.20260117",
	}
	for i, name := range matchingFiles {
		filePath := filepath.Join(tempDir, name)
		f, _ := os.Create(filePath)
		f.Close()
		modTime := time.Now().Add(-time.Duration(3-i) * 24 * time.Hour)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Clean up, keep only 1 rotated file
	cleanOldLogFiles(tempDir, baseLogFile, 1)

	// Verify non-matching files still exist
	for _, name := range nonMatchingFiles {
		filePath := filepath.Join(tempDir, name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Non-matching file %s should not be deleted", name)
		}
	}

	// Verify current log file still exists
	if _, err := os.Stat(baseLogFile); os.IsNotExist(err) {
		t.Error("Current log file should not be deleted")
	}

	// Verify only newest rotated file exists
	if _, err := os.Stat(filepath.Join(tempDir, "app.log.20260117")); os.IsNotExist(err) {
		t.Error("Newest rotated file should still exist")
	}
}

func TestCleanOldLogFiles_HourlyRotation(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create hourly rotated files (format: YYYYMMDDHH)
	hourlyFiles := []string{
		"app.log.2026011900",
		"app.log.2026011901",
		"app.log.2026011902",
		"app.log.2026011903",
		"app.log.2026011904",
	}

	for i, name := range hourlyFiles {
		filePath := filepath.Join(tempDir, name)
		f, _ := os.Create(filePath)
		f.Close()
		modTime := time.Now().Add(-time.Duration(5-i) * time.Hour)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Keep only 2 files
	cleanOldLogFiles(tempDir, baseLogFile, 2)

	entries, _ := os.ReadDir(tempDir)
	// Should have: app.log + 2 newest rotated = 3 files
	if len(entries) != 3 {
		t.Errorf("Expected 3 files after cleanup, got %d", len(entries))
	}
}

func TestCleanOldLogFiles_MinuteRotation(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "log_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "app.log")
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create minute rotated files (format: YYYYMMDDHHMMSS)
	minuteFiles := []string{
		"app.log.20260119120000",
		"app.log.20260119120500",
		"app.log.20260119121000",
		"app.log.20260119121500",
	}

	for i, name := range minuteFiles {
		filePath := filepath.Join(tempDir, name)
		f, _ := os.Create(filePath)
		f.Close()
		modTime := time.Now().Add(-time.Duration(4-i) * 5 * time.Minute)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Keep only 2 files
	cleanOldLogFiles(tempDir, baseLogFile, 2)

	entries, _ := os.ReadDir(tempDir)
	// Should have: app.log + 2 newest rotated = 3 files
	if len(entries) != 3 {
		t.Errorf("Expected 3 files after cleanup, got %d", len(entries))
	}
}

func TestCleanOldLogFiles_PanicLog(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "panic_cleanup_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseLogFile := filepath.Join(tempDir, "panic.log")
	f, _ := os.Create(baseLogFile)
	f.Close()

	// Create rotated panic log files
	panicFiles := []string{
		"panic.log.20260115",
		"panic.log.20260116",
		"panic.log.20260117",
		"panic.log.20260118",
	}

	for i, name := range panicFiles {
		filePath := filepath.Join(tempDir, name)
		f, _ := os.Create(filePath)
		f.Close()
		modTime := time.Now().Add(-time.Duration(4-i) * 24 * time.Hour)
		os.Chtimes(filePath, modTime, modTime)
	}

	// Keep only 2 files
	cleanOldLogFiles(tempDir, baseLogFile, 2)

	entries, _ := os.ReadDir(tempDir)
	// Should have: panic.log + 2 newest rotated = 3 files
	if len(entries) != 3 {
		t.Errorf("Expected 3 files after cleanup, got %d", len(entries))
	}

	// Verify newest files exist
	if _, err := os.Stat(filepath.Join(tempDir, "panic.log.20260117")); os.IsNotExist(err) {
		t.Error("panic.log.20260117 should still exist")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "panic.log.20260118")); os.IsNotExist(err) {
		t.Error("panic.log.20260118 should still exist")
	}
}