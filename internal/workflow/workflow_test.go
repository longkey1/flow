package workflow

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseWorkflow(t *testing.T, input string) *Workflow {
	t.Helper()
	var wf Workflow
	if err := yaml.Unmarshal([]byte(input), &wf); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	return &wf
}

func TestParseNeedsString(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
  test:
    needs: build
    steps:
      - run: echo test
`)
	if len(wf.Jobs["test"].Needs) != 1 || wf.Jobs["test"].Needs[0] != "build" {
		t.Errorf("expected needs=[build], got %v", wf.Jobs["test"].Needs)
	}
}

func TestParseNeedsList(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
  lint:
    steps:
      - run: echo lint
  test:
    needs: [build, lint]
    steps:
      - run: echo test
`)
	needs := wf.Jobs["test"].Needs
	if len(needs) != 2 || needs[0] != "build" || needs[1] != "lint" {
		t.Errorf("expected needs=[build, lint], got %v", needs)
	}
}

func TestParseNeedsEmpty(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
`)
	if len(wf.Jobs["build"].Needs) != 0 {
		t.Errorf("expected no needs, got %v", wf.Jobs["build"].Needs)
	}
}

func TestValidateUnknownDependency(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    needs: nonexistent
    steps:
      - run: echo build
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown job") {
		t.Errorf("expected 'unknown job' error, got: %v", err)
	}
}

func TestValidateSelfReference(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    needs: build
    steps:
      - run: echo build
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for self-reference")
	}
	if !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("expected self-reference error, got: %v", err)
	}
}

func TestValidateCyclicDependency(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  a:
    needs: b
    steps:
      - run: echo a
  b:
    needs: a
    steps:
      - run: echo b
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for cyclic dependency")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected circular dependency error, got: %v", err)
	}
}

func TestResolveOrderLinearChain(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
  test:
    needs: build
    steps:
      - run: echo test
  deploy:
    needs: test
    steps:
      - run: echo deploy
`)
	if err := wf.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	expected := []string{"build", "test", "deploy"}
	if len(wf.JobOrder) != len(expected) {
		t.Fatalf("expected order %v, got %v", expected, wf.JobOrder)
	}
	for i, name := range expected {
		if wf.JobOrder[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, wf.JobOrder[i])
		}
	}
}

func TestResolveOrderDiamond(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
  lint:
    needs: build
    steps:
      - run: echo lint
  test:
    needs: build
    steps:
      - run: echo test
  deploy:
    needs: [lint, test]
    steps:
      - run: echo deploy
`)
	if err := wf.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	// build must come first, deploy must come last
	if wf.JobOrder[0] != "build" {
		t.Errorf("expected build first, got %q", wf.JobOrder[0])
	}
	if wf.JobOrder[len(wf.JobOrder)-1] != "deploy" {
		t.Errorf("expected deploy last, got %q", wf.JobOrder[len(wf.JobOrder)-1])
	}
	// lint and test must come between build and deploy
	lintIdx, testIdx := -1, -1
	for i, name := range wf.JobOrder {
		if name == "lint" {
			lintIdx = i
		}
		if name == "test" {
			testIdx = i
		}
	}
	if lintIdx < 1 || testIdx < 1 {
		t.Errorf("lint and test should be in middle positions, got order %v", wf.JobOrder)
	}
}

func TestParseStepId(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - id: get-version
        run: echo version
      - run: echo build
`)
	steps := wf.Jobs["build"].Steps
	if steps[0].Id != "get-version" {
		t.Errorf("expected id 'get-version', got %q", steps[0].Id)
	}
	if steps[1].Id != "" {
		t.Errorf("expected empty id, got %q", steps[1].Id)
	}
}

func TestValidateDuplicateStepId(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - id: my-step
        run: echo a
      - id: my-step
        run: echo b
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate step id")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("expected 'duplicate id' error, got: %v", err)
	}
}

func TestValidateInvalidStepId(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - id: "invalid id!"
        run: echo a
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for invalid step id")
	}
	if !strings.Contains(err.Error(), "invalid id") {
		t.Errorf("expected 'invalid id' error, got: %v", err)
	}
}

func TestParseWorkflowEnv(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
env:
  GLOBAL_VAR: value1
  ANOTHER: value2
jobs:
  build:
    steps:
      - run: echo build
`)
	if len(wf.Env) != 2 {
		t.Fatalf("expected 2 workflow env vars, got %d", len(wf.Env))
	}
	if wf.Env["GLOBAL_VAR"] != "value1" {
		t.Errorf("expected GLOBAL_VAR=value1, got %q", wf.Env["GLOBAL_VAR"])
	}
	if wf.Env["ANOTHER"] != "value2" {
		t.Errorf("expected ANOTHER=value2, got %q", wf.Env["ANOTHER"])
	}
}

func TestParseJobEnv(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    env:
      JOB_VAR: jobval
    steps:
      - run: echo build
`)
	if wf.Jobs["build"].Env["JOB_VAR"] != "jobval" {
		t.Errorf("expected JOB_VAR=jobval, got %q", wf.Jobs["build"].Env["JOB_VAR"])
	}
}

func TestParseStepEnv(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - env:
          STEP_VAR: stepval
        run: echo build
`)
	if wf.Jobs["build"].Steps[0].Env["STEP_VAR"] != "stepval" {
		t.Errorf("expected STEP_VAR=stepval, got %q", wf.Jobs["build"].Steps[0].Env["STEP_VAR"])
	}
}

func TestParseAllLevelEnv(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
env:
  GLOBAL: g
jobs:
  build:
    env:
      JOB: j
    steps:
      - env:
          STEP: s
        run: echo build
`)
	if wf.Env["GLOBAL"] != "g" {
		t.Errorf("expected workflow env GLOBAL=g, got %q", wf.Env["GLOBAL"])
	}
	if wf.Jobs["build"].Env["JOB"] != "j" {
		t.Errorf("expected job env JOB=j, got %q", wf.Jobs["build"].Env["JOB"])
	}
	if wf.Jobs["build"].Steps[0].Env["STEP"] != "s" {
		t.Errorf("expected step env STEP=s, got %q", wf.Jobs["build"].Steps[0].Env["STEP"])
	}
}

func TestResolveOrderIndependentJobs(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  a:
    steps:
      - run: echo a
  b:
    steps:
      - run: echo b
  c:
    needs: a
    steps:
      - run: echo c
`)
	if err := wf.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	// a must come before c; b is independent
	aIdx, cIdx := -1, -1
	for i, name := range wf.JobOrder {
		if name == "a" {
			aIdx = i
		}
		if name == "c" {
			cIdx = i
		}
	}
	if aIdx >= cIdx {
		t.Errorf("expected a before c, got order %v", wf.JobOrder)
	}
}

func TestParseInputs(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
inputs:
  name:
    description: "Who to greet"
    required: true
  greeting:
    description: "Greeting message"
    default: "Hello"
jobs:
  greet:
    steps:
      - run: echo hello
`)
	if len(wf.Inputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(wf.Inputs))
	}
	nameInput := wf.Inputs["name"]
	if nameInput.Description != "Who to greet" {
		t.Errorf("expected description 'Who to greet', got %q", nameInput.Description)
	}
	if !nameInput.Required {
		t.Error("expected name input to be required")
	}
	greetingInput := wf.Inputs["greeting"]
	if greetingInput.Default != "Hello" {
		t.Errorf("expected default 'Hello', got %q", greetingInput.Default)
	}
}

func TestParseInputsEmpty(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
`)
	if len(wf.Inputs) != 0 {
		t.Errorf("expected no inputs, got %d", len(wf.Inputs))
	}
}

func TestValidateInvalidInputName(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
inputs:
  "invalid name!":
    description: "bad"
jobs:
  build:
    steps:
      - run: echo build
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for invalid input name")
	}
	if !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("expected 'invalid name' error, got: %v", err)
	}
}

func TestValidateValidInputNames(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
inputs:
  my-input:
    description: "with hyphen"
  my_input:
    description: "with underscore"
  myInput123:
    description: "alphanumeric"
jobs:
  build:
    steps:
      - run: echo build
`)
	if err := wf.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestParseStepUses(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - id: my-step
        uses: ./my-action
        with:
          name: "Claude"
`)
	step := wf.Jobs["build"].Steps[0]
	if step.Uses != "./my-action" {
		t.Errorf("expected uses './my-action', got %q", step.Uses)
	}
	if step.With["name"] != "Claude" {
		t.Errorf("expected with name='Claude', got %q", step.With["name"])
	}
}

func TestValidateRunAndUsesBothError(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo hello
        uses: ./my-action
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for both run and uses")
	}
	if !strings.Contains(err.Error(), "cannot have both run and uses") {
		t.Errorf("expected 'cannot have both run and uses' error, got: %v", err)
	}
}

func TestValidateNoRunNoUsesError(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - id: empty
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for step without run or uses")
	}
	if !strings.Contains(err.Error(), "must have a run command or uses reference") {
		t.Errorf("expected 'must have a run command or uses reference' error, got: %v", err)
	}
}

func TestValidateWithWithoutUsesError(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo hello
        with:
          key: value
`)
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected error for with without uses")
	}
	if !strings.Contains(err.Error(), "has with but no uses") {
		t.Errorf("expected 'has with but no uses' error, got: %v", err)
	}
}

func TestValidateUsesOnly(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - uses: ./my-action
`)
	if err := wf.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestParseJobOutputs(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    outputs:
      version: ${{ steps.get-ver.outputs.version }}
      artifact: ${{ steps.get-ver.outputs.artifact }}
    steps:
      - id: get-ver
        run: echo "version=1.0" >> $FLOW_OUTPUT
`)
	outputs := wf.Jobs["build"].Outputs
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}
	if outputs["version"] != "${{ steps.get-ver.outputs.version }}" {
		t.Errorf("expected version expression, got %q", outputs["version"])
	}
	if outputs["artifact"] != "${{ steps.get-ver.outputs.artifact }}" {
		t.Errorf("expected artifact expression, got %q", outputs["artifact"])
	}
}

func TestParseJobOutputsEmpty(t *testing.T) {
	wf := parseWorkflow(t, `
name: test
jobs:
  build:
    steps:
      - run: echo build
`)
	if len(wf.Jobs["build"].Outputs) != 0 {
		t.Errorf("expected no outputs, got %d", len(wf.Jobs["build"].Outputs))
	}
}
