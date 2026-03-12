package runner

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPrefixedWriterCompleteLine(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "build/Compile", false)
	pw.Write([]byte("hello world\n"))

	data, _ := os.ReadFile(lf.Path())
	line := string(data)
	if !strings.Contains(line, "[build/Compile] hello world") {
		t.Errorf("expected prefixed line, got: %s", line)
	}
	// Should have timestamp
	if !strings.Contains(line, "T") {
		t.Errorf("expected timestamp in line, got: %s", line)
	}
}

func TestPrefixedWriterStderr(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "build/Deploy", true)
	pw.Write([]byte("error occurred\n"))

	data, _ := os.ReadFile(lf.Path())
	line := string(data)
	if !strings.Contains(line, "[build/Deploy] [stderr] error occurred") {
		t.Errorf("expected stderr prefix, got: %s", line)
	}
}

func TestPrefixedWriterPartialLines(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "job/step", false)

	// Write partial line
	pw.Write([]byte("hel"))
	data, _ := os.ReadFile(lf.Path())
	if len(data) != 0 {
		t.Errorf("expected no output for partial line, got: %s", data)
	}

	// Complete the line
	pw.Write([]byte("lo\n"))
	data, _ = os.ReadFile(lf.Path())
	if !strings.Contains(string(data), "[job/step] hello") {
		t.Errorf("expected complete line after newline, got: %s", data)
	}
}

func TestPrefixedWriterMultipleLines(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "job/step", false)
	pw.Write([]byte("line1\nline2\nline3\n"))

	data, _ := os.ReadFile(lf.Path())
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %s", len(lines), data)
	}
	if !strings.Contains(lines[0], "line1") {
		t.Errorf("expected line1, got: %s", lines[0])
	}
	if !strings.Contains(lines[2], "line3") {
		t.Errorf("expected line3, got: %s", lines[2])
	}
}

func TestPrefixedWriterFlush(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "job/step", false)
	pw.Write([]byte("no newline"))

	// Should have no output yet
	data, _ := os.ReadFile(lf.Path())
	if len(data) != 0 {
		t.Errorf("expected no output before flush, got: %s", data)
	}

	// Flush should write the remaining buffer
	pw.Flush()
	data, _ = os.ReadFile(lf.Path())
	if !strings.Contains(string(data), "[job/step] no newline") {
		t.Errorf("expected flushed line, got: %s", data)
	}
}

func TestPrefixedWriterFlushEmpty(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "job/step", false)
	if err := pw.Flush(); err != nil {
		t.Errorf("flush on empty buffer should not error, got: %v", err)
	}
}

func TestPrefixedWriterConcurrent(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	pw := NewPrefixedWriter(lf, "job/step", false)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				pw.Write([]byte("line\n"))
			}
		}(i)
	}
	wg.Wait()
	pw.Flush()

	data, _ := os.ReadFile(lf.Path())
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 100 {
		t.Errorf("expected 100 lines from concurrent writes, got %d", len(lines))
	}
}

func TestLogFileCreate(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")

	lf, err := NewLogFile(logsDir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()

	path := lf.Path()
	if !strings.Contains(path, "deploy-") {
		t.Errorf("expected filename to contain 'deploy-', got: %s", path)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Errorf("expected .log suffix, got: %s", path)
	}

	// Directory should have been created
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Errorf("expected logs directory to be created")
	}
}

func TestLogFileWrite(t *testing.T) {
	dir := t.TempDir()
	lf, err := NewLogFile(dir, "test")
	if err != nil {
		t.Fatal(err)
	}

	lf.Write([]byte("hello"))
	lf.Write([]byte(" world"))
	lf.Close()

	data, _ := os.ReadFile(lf.Path())
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got: %s", data)
	}
}

func TestRotateLogsKeepsNewest(t *testing.T) {
	dir := t.TempDir()

	// Create 5 log files with different times
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "test-"+strings.Repeat("0", i)+".log")
		os.WriteFile(name, []byte("data"), 0o644)
		// Set modification time so files are ordered
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		os.Chtimes(name, modTime, modTime)
	}

	if err := RotateLogs(dir, 3); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	var logFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e.Name())
		}
	}

	if len(logFiles) != 3 {
		t.Errorf("expected 3 log files after rotation, got %d: %v", len(logFiles), logFiles)
	}
}

func TestRotateLogsNoopWhenUnderLimit(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, "test-"+strings.Repeat("0", i)+".log")
		os.WriteFile(name, []byte("data"), 0o644)
	}

	if err := RotateLogs(dir, 5); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	var logFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e.Name())
		}
	}

	if len(logFiles) != 3 {
		t.Errorf("expected 3 log files (no rotation needed), got %d", len(logFiles))
	}
}

func TestRotateLogsIgnoresNonLogFiles(t *testing.T) {
	dir := t.TempDir()

	// Create log files and non-log files
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "test-"+strings.Repeat("0", i)+".log")
		os.WriteFile(name, []byte("data"), 0o644)
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		os.Chtimes(name, modTime, modTime)
	}
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("keep"), 0o644)

	if err := RotateLogs(dir, 2); err != nil {
		t.Fatal(err)
	}

	// Non-log file should still exist
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); os.IsNotExist(err) {
		t.Errorf("expected non-log file to be preserved")
	}

	entries, _ := os.ReadDir(dir)
	var logFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, e.Name())
		}
	}
	if len(logFiles) != 2 {
		t.Errorf("expected 2 log files after rotation, got %d", len(logFiles))
	}
}

func TestRotateLogsMissingDir(t *testing.T) {
	err := RotateLogs("/nonexistent/path/logs", 5)
	if err != nil {
		t.Errorf("expected no error for missing directory, got: %v", err)
	}
}
