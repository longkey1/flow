package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorkflowYAML = `name: deploy
jobs:
  build:
    steps:
      - run: echo build
`

func TestLoadWorkflow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.yaml")
	if err := os.WriteFile(path, []byte(validWorkflowYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	wf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "deploy" {
		t.Errorf("expected name 'deploy', got %q", wf.Name)
	}
	if len(wf.JobOrder) != 1 || wf.JobOrder[0] != "build" {
		t.Errorf("expected job order [build], got %v", wf.JobOrder)
	}
}

func TestLoadWorkflowNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(filepath.Join(dir, "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected not-exist error, got: %v", err)
	}
}

func TestLoadWorkflowInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("name: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parsing workflow") {
		t.Errorf("expected 'parsing workflow' error, got: %v", err)
	}
}

func TestLoadWorkflowValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	content := `jobs:
  build:
    steps:
      - run: echo build
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validating workflow") {
		t.Errorf("expected 'validating workflow' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got: %v", err)
	}
}

func TestFindWorkflowYaml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := Find(dir, "deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := dir + "/deploy.yaml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestFindWorkflowYml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deploy.yml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := Find(dir, "deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := dir + "/deploy.yml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestFindWorkflowPrefersYaml(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"deploy.yaml", "deploy.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path, err := Find(dir, "deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, ".yaml") {
		t.Errorf("expected .yaml to take precedence, got %q", path)
	}
}

func TestFindWorkflowNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := Find(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing workflow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}
