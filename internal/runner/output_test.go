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
