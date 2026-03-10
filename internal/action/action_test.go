package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseAction(t *testing.T, input string) *Action {
	t.Helper()
	var a Action
	if err := yaml.Unmarshal([]byte(input), &a); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return &a
}

func TestParseAction(t *testing.T) {
	a := parseAction(t, `
name: my-action
description: "A test action"
inputs:
  name:
    description: "Who to greet"
    required: true
    default: "world"
  greeting:
    description: "Greeting prefix"
outputs:
  result:
    description: "The result"
runs:
  steps:
    - id: greet
      name: Generate greeting
      run: echo "result=hello" >> $FLOW_OUTPUT
    - run: echo done
`)
	if a.Name != "my-action" {
		t.Errorf("expected name 'my-action', got %q", a.Name)
	}
	if a.Description != "A test action" {
		t.Errorf("expected description 'A test action', got %q", a.Description)
	}
	if len(a.Inputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(a.Inputs))
	}
	nameInput := a.Inputs["name"]
	if !nameInput.Required {
		t.Error("expected name input to be required")
	}
	if nameInput.Default != "world" {
		t.Errorf("expected default 'world', got %q", nameInput.Default)
	}
	if len(a.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(a.Outputs))
	}
	if a.Outputs["result"].Description != "The result" {
		t.Errorf("expected output description 'The result', got %q", a.Outputs["result"].Description)
	}
	if len(a.Runs.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(a.Runs.Steps))
	}
	if a.Runs.Steps[0].Id != "greet" {
		t.Errorf("expected step id 'greet', got %q", a.Runs.Steps[0].Id)
	}
	if a.Runs.Steps[0].Name != "Generate greeting" {
		t.Errorf("expected step name 'Generate greeting', got %q", a.Runs.Steps[0].Name)
	}
}

func TestValidateNameRequired(t *testing.T) {
	a := parseAction(t, `
runs:
  steps:
    - run: echo hello
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got: %v", err)
	}
}

func TestValidateStepsRequired(t *testing.T) {
	a := parseAction(t, `
name: test
runs:
  steps: []
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for empty steps")
	}
	if !strings.Contains(err.Error(), "at least one step") {
		t.Errorf("expected 'at least one step' error, got: %v", err)
	}
}

func TestValidateStepRunRequired(t *testing.T) {
	a := parseAction(t, `
name: test
runs:
  steps:
    - id: no-run
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for step without run")
	}
	if !strings.Contains(err.Error(), "run command") {
		t.Errorf("expected 'run command' error, got: %v", err)
	}
}

func TestValidateDuplicateStepId(t *testing.T) {
	a := parseAction(t, `
name: test
runs:
  steps:
    - id: same
      run: echo a
    - id: same
      run: echo b
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate step id")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("expected 'duplicate id' error, got: %v", err)
	}
}

func TestValidateInvalidInputName(t *testing.T) {
	a := parseAction(t, `
name: test
inputs:
  "invalid name!":
    description: "bad"
runs:
  steps:
    - run: echo hello
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for invalid input name")
	}
	if !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("expected 'invalid name' error, got: %v", err)
	}
}

func TestValidateInvalidStepId(t *testing.T) {
	a := parseAction(t, `
name: test
runs:
  steps:
    - id: "bad id!"
      run: echo hello
`)
	err := a.Validate()
	if err == nil {
		t.Fatal("expected error for invalid step id")
	}
	if !strings.Contains(err.Error(), "invalid id") {
		t.Errorf("expected 'invalid id' error, got: %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	actionDir := filepath.Join(dir, "my-action")
	os.MkdirAll(actionDir, 0o755)
	content := `name: my-action
runs:
  steps:
    - run: echo hello
`
	os.WriteFile(filepath.Join(actionDir, "action.yaml"), []byte(content), 0o644)

	a, err := Load(filepath.Join(actionDir, "action.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Name != "my-action" {
		t.Errorf("expected name 'my-action', got %q", a.Name)
	}
}

func TestLoadValidationError(t *testing.T) {
	dir := t.TempDir()
	content := `runs:
  steps:
    - run: echo hello
`
	path := filepath.Join(dir, "action.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got: %v", err)
	}
}

func TestFindYaml(t *testing.T) {
	dir := t.TempDir()
	actionDir := filepath.Join(dir, "my-action")
	os.MkdirAll(actionDir, 0o755)
	os.WriteFile(filepath.Join(actionDir, "action.yaml"), []byte(""), 0o644)

	path, err := Find(dir, "my-action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(actionDir, "action.yaml")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestFindYml(t *testing.T) {
	dir := t.TempDir()
	actionDir := filepath.Join(dir, "my-action")
	os.MkdirAll(actionDir, 0o755)
	os.WriteFile(filepath.Join(actionDir, "action.yml"), []byte(""), 0o644)

	path, err := Find(dir, "my-action")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(actionDir, "action.yml")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestFindNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := Find(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing action")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestParseStepEnv(t *testing.T) {
	a := parseAction(t, `
name: test
runs:
  steps:
    - run: echo hello
      env:
        MY_VAR: value
`)
	if a.Runs.Steps[0].Env["MY_VAR"] != "value" {
		t.Errorf("expected MY_VAR=value, got %q", a.Runs.Steps[0].Env["MY_VAR"])
	}
}
