package runner

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogFile manages a single log file for a workflow run.
type LogFile struct {
	file *os.File
	mu   sync.Mutex
}

// NewLogFile creates the logs directory and a new timestamped log file.
func NewLogFile(logsDir, workflowName string) (*LogFile, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating logs directory: %w", err)
	}

	now := time.Now()
	filename := fmt.Sprintf("%s-%s.log", workflowName, now.Format("20060102-150405"))
	path := filepath.Join(logsDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	return &LogFile{file: f}, nil
}

// Write writes data to the log file. Safe for concurrent use.
func (lf *LogFile) Write(p []byte) (int, error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.file.Write(p)
}

// Path returns the file path of the log file.
func (lf *LogFile) Path() string {
	return lf.file.Name()
}

// Close closes the log file.
func (lf *LogFile) Close() error {
	return lf.file.Close()
}

// PrefixedWriter is an io.Writer that buffers incomplete lines and writes
// complete lines with a timestamp and prefix.
type PrefixedWriter struct {
	prefix string
	dest   *LogFile
	buf    []byte
	mu     sync.Mutex
}

// NewPrefixedWriter creates a new PrefixedWriter.
func NewPrefixedWriter(dest *LogFile, prefix string) *PrefixedWriter {
	return &PrefixedWriter{
		prefix: prefix,
		dest:   dest,
	}
}

// Write implements io.Writer. Buffers partial lines and writes complete lines with prefix.
func (pw *PrefixedWriter) Write(p []byte) (int, error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.buf = append(pw.buf, p...)

	for {
		idx := bytes.IndexByte(pw.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(pw.buf[:idx])
		pw.buf = pw.buf[idx+1:]

		if err := pw.writeLine(line); err != nil {
			return len(p), err
		}
	}

	return len(p), nil
}

// Flush writes any remaining buffered data as a complete line.
func (pw *PrefixedWriter) Flush() error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if len(pw.buf) == 0 {
		return nil
	}

	line := string(pw.buf)
	pw.buf = nil
	return pw.writeLine(line)
}

func (pw *PrefixedWriter) writeLine(line string) error {
	timestamp := time.Now().Format(time.RFC3339)
	formatted := fmt.Sprintf("%s [%s] %s\n", timestamp, pw.prefix, line)
	_, err := pw.dest.Write([]byte(formatted))
	return err
}

// RotateLogs removes old log files, keeping at most maxRuns files.
func RotateLogs(logsDir string, maxRuns int) error {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	type logEntry struct {
		name    string
		modTime time.Time
	}
	var logs []logEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logEntry{name: e.Name(), modTime: info.ModTime()})
	}

	if len(logs) <= maxRuns {
		return nil
	}

	// Sort by mod time descending (newest first)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].modTime.After(logs[j].modTime)
	})

	// Delete files beyond maxRuns
	for _, l := range logs[maxRuns:] {
		os.Remove(filepath.Join(logsDir, l.name))
	}

	return nil
}
