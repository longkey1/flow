package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOutputFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	if err := os.WriteFile(path, []byte("key1=value1\nkey2=hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["key1"] != "value1" {
		t.Errorf("expected key1=value1, got %q", outputs["key1"])
	}
	if outputs["key2"] != "hello world" {
		t.Errorf("expected key2=hello world, got %q", outputs["key2"])
	}
}

func TestParseOutputFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outputs) != 0 {
		t.Errorf("expected empty outputs, got %v", outputs)
	}
}

func TestParseOutputFileValueWithEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	if err := os.WriteFile(path, []byte("url=https://example.com?a=1&b=2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["url"] != "https://example.com?a=1&b=2" {
		t.Errorf("expected full URL, got %q", outputs["url"])
	}
}

func TestParseOutputFileSkipsInvalidLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	if err := os.WriteFile(path, []byte("valid=yes\nno-equals-here\n=empty-key\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d: %v", len(outputs), outputs)
	}
	if outputs["valid"] != "yes" {
		t.Errorf("expected valid=yes, got %q", outputs["valid"])
	}
}

func TestParseOutputFileDelimiterMultiline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "body<<EOF\nline1\nline2\nline3\nEOF\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["body"] != "line1\nline2\nline3" {
		t.Errorf("expected multiline value, got %q", outputs["body"])
	}
}

func TestParseOutputFileDelimiterSingleLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "msg<<END\nhello\nEND\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["msg"] != "hello" {
		t.Errorf("expected 'hello', got %q", outputs["msg"])
	}
}

func TestParseOutputFileDelimiterEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "empty<<EOF\nEOF\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["empty"] != "" {
		t.Errorf("expected empty string, got %q", outputs["empty"])
	}
}

func TestParseOutputFileDelimiterMixedWithKeyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "simple=value\nbody<<EOF\nline1\nline2\nEOF\nother=val2\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["simple"] != "value" {
		t.Errorf("expected simple=value, got %q", outputs["simple"])
	}
	if outputs["body"] != "line1\nline2" {
		t.Errorf("expected multiline body, got %q", outputs["body"])
	}
	if outputs["other"] != "val2" {
		t.Errorf("expected other=val2, got %q", outputs["other"])
	}
}

func TestParseOutputFileDelimiterValueContainsEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "data<<DELIM\nkey=value\na=b=c\nDELIM\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["data"] != "key=value\na=b=c" {
		t.Errorf("expected value with equals, got %q", outputs["data"])
	}
}

func TestParseOutputFileCustomDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "msg<<ghadelimiter_abc123\nhello world\nghadelimiter_abc123\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["msg"] != "hello world" {
		t.Errorf("expected 'hello world', got %q", outputs["msg"])
	}
}

func TestParseOutputFileUnclosedDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	content := "body<<EOF\nline1\nline2\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	outputs, err := parseOutputFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputs["body"] != "line1\nline2" {
		t.Errorf("expected collected lines, got %q", outputs["body"])
	}
}
