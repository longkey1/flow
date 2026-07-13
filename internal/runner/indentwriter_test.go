package runner

import (
	"bytes"
	"errors"
	"testing"
)

func TestIndentWriterSingleLine(t *testing.T) {
	var buf bytes.Buffer
	iw := newIndentWriter(&buf, "  ")

	n, err := iw.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 6 {
		t.Errorf("expected n=6, got %d", n)
	}
	if got := buf.String(); got != "  hello\n" {
		t.Errorf("expected %q, got %q", "  hello\n", got)
	}
}

func TestIndentWriterMultipleLines(t *testing.T) {
	var buf bytes.Buffer
	iw := newIndentWriter(&buf, "> ")

	if _, err := iw.Write([]byte("a\nb\nc\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := buf.String(); got != "> a\n> b\n> c\n" {
		t.Errorf("expected %q, got %q", "> a\n> b\n> c\n", got)
	}
}

func TestIndentWriterPartialWrites(t *testing.T) {
	var buf bytes.Buffer
	iw := newIndentWriter(&buf, "  ")

	// A line split across multiple Write calls should only be prefixed once.
	for _, chunk := range []string{"par", "tial", "\nnext\n"} {
		if _, err := iw.Write([]byte(chunk)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := buf.String(); got != "  partial\n  next\n" {
		t.Errorf("expected %q, got %q", "  partial\n  next\n", got)
	}
}

func TestIndentWriterNoTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	iw := newIndentWriter(&buf, "  ")

	n, err := iw.Write([]byte("no newline"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len("no newline") {
		t.Errorf("expected n=%d, got %d", len("no newline"), n)
	}
	if got := buf.String(); got != "  no newline" {
		t.Errorf("expected %q, got %q", "  no newline", got)
	}
}

func TestIndentWriterEmptyWrite(t *testing.T) {
	var buf bytes.Buffer
	iw := newIndentWriter(&buf, "  ")

	n, err := iw.Write(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

// failAfterWriter fails once more than failAfter successful writes have occurred.
type failAfterWriter struct {
	failAfter int
	writes    int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed")
	}
	w.writes++
	return len(p), nil
}

func TestIndentWriterPrefixWriteError(t *testing.T) {
	iw := newIndentWriter(&failAfterWriter{failAfter: 0}, "  ")

	n, err := iw.Write([]byte("hello\n"))
	if err == nil {
		t.Fatal("expected error from prefix write")
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
}

func TestIndentWriterContentWriteError(t *testing.T) {
	// First write (prefix) succeeds, second write (line content) fails.
	iw := newIndentWriter(&failAfterWriter{failAfter: 1}, "  ")

	if _, err := iw.Write([]byte("hello\n")); err == nil {
		t.Fatal("expected error from content write")
	}
}
