package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/longkey1/flow/internal/workflow"
)

func writeAction(t *testing.T, dir, name, content string) {
	t.Helper()
	actionDir := filepath.Join(dir, name)
	os.MkdirAll(actionDir, 0o755)
	os.WriteFile(filepath.Join(actionDir, "action.yaml"), []byte(content), 0o644)
}

func makeWorkflow(t *testing.T, jobs map[string]workflow.Job, order []string) *workflow.Workflow {
	t.Helper()
	wf := &workflow.Workflow{
		Name:     "test",
		Jobs:     jobs,
		JobOrder: order,
	}
	return wf
}

func TestRunNoDependencies(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"a": {Steps: []workflow.Step{{Run: "echo a"}}},
		"b": {Steps: []workflow.Step{{Run: "echo b"}}},
	}, []string{"a", "b"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "=== Job: a ===") || !strings.Contains(out, "=== Job: b ===") {
		t.Errorf("expected both jobs to run, got:\n%s", out)
	}
}

func TestRunWithDependenciesAllSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build":  {Steps: []workflow.Step{{Run: "echo build"}}},
		"test":   {Needs: []string{"build"}, Steps: []workflow.Step{{Run: "echo test"}}},
		"deploy": {Needs: []string{"test"}, Steps: []workflow.Step{{Run: "echo deploy"}}},
	}, []string{"build", "test", "deploy"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	buildIdx := strings.Index(out, "=== Job: build ===")
	testIdx := strings.Index(out, "=== Job: test ===")
	deployIdx := strings.Index(out, "=== Job: deploy ===")
	if buildIdx >= testIdx || testIdx >= deployIdx {
		t.Errorf("expected build < test < deploy order, got:\n%s", out)
	}
}

func TestRunDependencyFailedSkipsDependents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{{Run: "exit 1"}}},
		"test":  {Needs: []string{"build"}, Steps: []workflow.Step{{Run: "echo test"}}},
	}, []string{"build", "test"})

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error when job fails")
	}
	out := stdout.String()
	if !strings.Contains(out, "(skipped)") {
		t.Errorf("expected test to be skipped, got:\n%s", out)
	}
}

func TestRunIndependentJobsRunDespiteFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build":       {Steps: []workflow.Step{{Run: "exit 1"}}},
		"independent": {Steps: []workflow.Step{{Run: "echo independent"}}},
		"test":        {Needs: []string{"build"}, Steps: []workflow.Step{{Run: "echo test"}}},
	}, []string{"build", "independent", "test"})

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error when job fails")
	}
	out := stdout.String()
	if !strings.Contains(out, "=== Job: independent ===") {
		t.Errorf("expected independent job to run, got:\n%s", out)
	}
	if !strings.Contains(out, "(skipped)") {
		t.Errorf("expected test to be skipped, got:\n%s", out)
	}
}

func TestRunStepOutputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Id: "producer", Run: `echo "greeting=hello world" >> $FLOW_OUTPUT`},
			{Run: `echo "${{ steps.producer.outputs.greeting }}"`},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello world") {
		t.Errorf("expected 'hello world' in output, got:\n%s", stdout.String())
	}
}

func TestRunStepOutputsMultipleKeys(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Id: "info", Run: `printf "name=flow\nversion=2.0\n" >> $FLOW_OUTPUT`},
			{Run: `echo "${{ steps.info.outputs.name }}-${{ steps.info.outputs.version }}"`},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "flow-2.0") {
		t.Errorf("expected 'flow-2.0' in output, got:\n%s", stdout.String())
	}
}

func TestRunStepOutputsUnknownStepReturnsEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Run: `echo "[${{ steps.nonexistent.outputs.key }}]"`},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "[]") {
		t.Errorf("expected '[]' in output, got:\n%s", stdout.String())
	}
}

func TestRunWorkflowEnv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Env:  map[string]string{"WF_VAR": "from_workflow"},
		Jobs: map[string]workflow.Job{
			"build": {Steps: []workflow.Step{{Run: `echo "$WF_VAR"`}}},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "from_workflow") {
		t.Errorf("expected 'from_workflow' in output, got:\n%s", stdout.String())
	}
}

func TestRunJobEnv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Jobs: map[string]workflow.Job{
			"build": {
				Env:   map[string]string{"JOB_VAR": "from_job"},
				Steps: []workflow.Step{{Run: `echo "$JOB_VAR"`}},
			},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "from_job") {
		t.Errorf("expected 'from_job' in output, got:\n%s", stdout.String())
	}
}

func TestRunStepEnv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Jobs: map[string]workflow.Job{
			"build": {Steps: []workflow.Step{
				{Env: map[string]string{"STEP_VAR": "from_step"}, Run: `echo "$STEP_VAR"`},
			}},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "from_step") {
		t.Errorf("expected 'from_step' in output, got:\n%s", stdout.String())
	}
}

func TestRunEnvOverride(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Env:  map[string]string{"MY_VAR": "workflow_val"},
		Jobs: map[string]workflow.Job{
			"build": {
				Env: map[string]string{"MY_VAR": "job_val"},
				Steps: []workflow.Step{
					{Env: map[string]string{"MY_VAR": "step_val"}, Run: `echo "$MY_VAR"`},
				},
			},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.Contains(out, "step_val") {
		t.Errorf("expected step env to override, got:\n%s", out)
	}
}

func TestRunEnvJobOverridesWorkflow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Env:  map[string]string{"MY_VAR": "workflow_val"},
		Jobs: map[string]workflow.Job{
			"build": {
				Env:   map[string]string{"MY_VAR": "job_val"},
				Steps: []workflow.Step{{Run: `echo "$MY_VAR"`}},
			},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.Contains(out, "job_val") {
		t.Errorf("expected job env to override workflow, got:\n%s", out)
	}
}

func TestRunEnvAllLevelsMerged(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Env:  map[string]string{"WF": "w"},
		Jobs: map[string]workflow.Job{
			"build": {
				Env: map[string]string{"JB": "j"},
				Steps: []workflow.Step{
					{Env: map[string]string{"ST": "s"}, Run: `echo "$WF $JB $ST"`},
				},
			},
		},
		JobOrder: []string{"build"},
	}

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "w j s") {
		t.Errorf("expected 'w j s' in output, got:\n%s", stdout.String())
	}
}

func TestRunTransitiveSkip(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"a": {Steps: []workflow.Step{{Run: "exit 1"}}},
		"b": {Needs: []string{"a"}, Steps: []workflow.Step{{Run: "echo b"}}},
		"c": {Needs: []string{"b"}, Steps: []workflow.Step{{Run: "echo c"}}},
	}, []string{"a", "b", "c"})

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error when job fails")
	}
	out := stdout.String()
	if !strings.Contains(out, "=== Job: b (skipped) ===") {
		t.Errorf("expected b to be skipped, got:\n%s", out)
	}
	if !strings.Contains(out, "=== Job: c (skipped) ===") {
		t.Errorf("expected c to be skipped, got:\n%s", out)
	}
}

func TestRunWithInputs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Inputs: map[string]workflow.Input{
			"name": {Description: "Who to greet", Required: true},
		},
		Jobs: map[string]workflow.Job{
			"greet": {Steps: []workflow.Step{{Run: `echo "${{ inputs.name }}"`}}},
		},
		JobOrder: []string{"greet"},
	}

	if err := r.Run(wf, map[string]string{"name": "World"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "World") {
		t.Errorf("expected 'World' in output, got:\n%s", stdout.String())
	}
}

func TestRunRequiredInputMissing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Inputs: map[string]workflow.Input{
			"name": {Required: true},
		},
		Jobs: map[string]workflow.Job{
			"greet": {Steps: []workflow.Step{{Run: `echo hello`}}},
		},
		JobOrder: []string{"greet"},
	}

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error for missing required input")
	}
	if !strings.Contains(err.Error(), "required input") {
		t.Errorf("expected 'required input' error, got: %v", err)
	}
}

func TestRunInputDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Inputs: map[string]workflow.Input{
			"greeting": {Default: "Hello"},
			"name":     {Required: true},
		},
		Jobs: map[string]workflow.Job{
			"greet": {Steps: []workflow.Step{{Run: `echo "${{ inputs.greeting }}, ${{ inputs.name }}!"`}}},
		},
		JobOrder: []string{"greet"},
	}

	if err := r.Run(wf, map[string]string{"name": "Alice"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Hello, Alice!") {
		t.Errorf("expected 'Hello, Alice!' in output, got:\n%s", stdout.String())
	}
}

func TestRunInputOverridesDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := &workflow.Workflow{
		Name: "test",
		Inputs: map[string]workflow.Input{
			"greeting": {Default: "Hello"},
		},
		Jobs: map[string]workflow.Job{
			"greet": {Steps: []workflow.Step{{Run: `echo "${{ inputs.greeting }}"`}}},
		},
		JobOrder: []string{"greet"},
	}

	if err := r.Run(wf, map[string]string{"greeting": "Hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Hi") {
		t.Errorf("expected 'Hi' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionBasic(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
runs:
  steps:
    - run: echo "hello from action"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Uses: "./greet"},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello from action") {
		t.Errorf("expected 'hello from action' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionInputsWithValues(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
inputs:
  name:
    description: "Who to greet"
    required: true
runs:
  steps:
    - run: echo "hello ${{ inputs.name }}"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Uses: "./greet", With: map[string]string{"name": "Claude"}},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello Claude") {
		t.Errorf("expected 'hello Claude' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionInputsDefault(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
inputs:
  name:
    default: "world"
runs:
  steps:
    - run: echo "hello ${{ inputs.name }}"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Uses: "./greet"},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello world") {
		t.Errorf("expected 'hello world' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionOutputs(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
inputs:
  name:
    required: true
outputs:
  greeting:
    description: "The greeting"
runs:
  steps:
    - id: greet
      run: echo "greeting=hello ${{ inputs.name }}" >> $FLOW_OUTPUT
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Id: "my-step", Uses: "./greet", With: map[string]string{"name": "Claude"}},
			{Run: `echo "${{ steps.my-step.outputs.greeting }}"`},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello Claude") {
		t.Errorf("expected 'hello Claude' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionStepOutputsWithinAction(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "multi", `
name: multi
runs:
  steps:
    - id: step1
      run: echo "val=from-step1" >> $FLOW_OUTPUT
    - run: echo "got ${{ steps.step1.outputs.val }}"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Uses: "./multi"},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "got from-step1") {
		t.Errorf("expected 'got from-step1' in output, got:\n%s", stdout.String())
	}
}

func TestRunActionRequiredInputMissing(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
inputs:
  name:
    required: true
runs:
  steps:
    - run: echo "hello ${{ inputs.name }}"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Uses: "./greet"},
		}},
	}, []string{"build"})

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error for missing required input")
	}
}

func TestRunActionWithExpressionExpansion(t *testing.T) {
	actionsDir := t.TempDir()
	writeAction(t, actionsDir, "greet", `
name: greet
inputs:
  name:
    required: true
runs:
  steps:
    - run: echo "hello ${{ inputs.name }}"
`)

	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.ActionsDir = actionsDir

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build": {Steps: []workflow.Step{
			{Id: "producer", Run: `echo "val=World" >> $FLOW_OUTPUT`},
			{Uses: "./greet", With: map[string]string{"name": "${{ steps.producer.outputs.val }}"}},
		}},
	}, []string{"build"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "hello World") {
		t.Errorf("expected 'hello World' in output, got:\n%s", stdout.String())
	}
}

func TestRunParallelIndependentJobs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")
	r.Quiet = true

	wf := makeWorkflow(t, map[string]workflow.Job{
		"a": {Steps: []workflow.Step{{Run: "sleep 0.5 && echo a"}}},
		"b": {Steps: []workflow.Step{{Run: "sleep 0.5 && echo b"}}},
		"c": {Steps: []workflow.Step{{Run: "sleep 0.5 && echo c"}}},
	}, []string{"a", "b", "c"})

	start := time.Now()
	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	// If run sequentially, would take ~1.5s; parallel should be ~0.5s
	if elapsed > 1200*time.Millisecond {
		t.Errorf("expected parallel execution (<1.2s), took %v", elapsed)
	}

	out := stdout.String()
	if !strings.Contains(out, "a") || !strings.Contains(out, "b") || !strings.Contains(out, "c") {
		t.Errorf("expected all jobs to produce output, got:\n%s", out)
	}
}

func TestRunParallelNoOutputInterleaving(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"a": {Steps: []workflow.Step{
			{Run: "echo a-line1"},
			{Run: "echo a-line2"},
		}},
		"b": {Steps: []workflow.Step{
			{Run: "echo b-line1"},
			{Run: "echo b-line2"},
		}},
	}, []string{"a", "b"})

	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Each job's output should be contiguous (not interleaved)
	aIdx1 := strings.Index(out, "a-line1")
	aIdx2 := strings.Index(out, "a-line2")
	bIdx1 := strings.Index(out, "b-line1")
	bIdx2 := strings.Index(out, "b-line2")

	if aIdx1 < 0 || aIdx2 < 0 || bIdx1 < 0 || bIdx2 < 0 {
		t.Fatalf("expected all output lines, got:\n%s", out)
	}

	// a's lines should be together, b's lines should be together
	aContiguous := (aIdx1 < aIdx2) && (aIdx2 < bIdx1 || bIdx2 < aIdx1)
	bContiguous := (bIdx1 < bIdx2) && (bIdx2 < aIdx1 || aIdx2 < bIdx1)
	if !aContiguous || !bContiguous {
		t.Errorf("expected non-interleaved output, got:\n%s", out)
	}
}

func TestRunParallelDiamondDependency(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"setup":  {Steps: []workflow.Step{{Run: "echo setup"}}},
		"lint":   {Needs: []string{"setup"}, Steps: []workflow.Step{{Run: "sleep 0.3 && echo lint"}}},
		"test":   {Needs: []string{"setup"}, Steps: []workflow.Step{{Run: "sleep 0.3 && echo test"}}},
		"deploy": {Needs: []string{"lint", "test"}, Steps: []workflow.Step{{Run: "echo deploy"}}},
	}, []string{"setup", "lint", "test", "deploy"})

	start := time.Now()
	if err := r.Run(wf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	// lint and test run in parallel after setup, then deploy
	// Sequential: ~0.9s; Parallel: ~0.6s
	if elapsed > 900*time.Millisecond {
		t.Errorf("expected diamond parallel execution (<0.9s), took %v", elapsed)
	}

	out := stdout.String()
	setupIdx := strings.Index(out, "setup")
	deployIdx := strings.Index(out, "deploy")
	lintIdx := strings.Index(out, "lint")
	testIdx := strings.Index(out, "test")
	if setupIdx < 0 || deployIdx < 0 || lintIdx < 0 || testIdx < 0 {
		t.Fatalf("expected all jobs output, got:\n%s", out)
	}
	if setupIdx > lintIdx || setupIdx > testIdx {
		t.Errorf("expected setup before lint and test, got:\n%s", out)
	}
	if lintIdx > deployIdx || testIdx > deployIdx {
		t.Errorf("expected lint and test before deploy, got:\n%s", out)
	}
}

func TestRunParallelFailedJobSkipsDependents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := New(nil, &stdout, &stderr, "")

	wf := makeWorkflow(t, map[string]workflow.Job{
		"build":  {Steps: []workflow.Step{{Run: "exit 1"}}},
		"test":   {Needs: []string{"build"}, Steps: []workflow.Step{{Run: "echo test"}}},
		"deploy": {Needs: []string{"test"}, Steps: []workflow.Step{{Run: "echo deploy"}}},
		"lint":   {Steps: []workflow.Step{{Run: "echo lint"}}},
	}, []string{"build", "test", "deploy", "lint"})

	err := r.Run(wf, nil)
	if err == nil {
		t.Fatal("expected error when job fails")
	}

	out := stdout.String()
	// lint (independent) should still run
	if !strings.Contains(out, "lint") {
		t.Errorf("expected lint to run, got:\n%s", out)
	}
	// test and deploy should be skipped
	if !strings.Contains(out, "=== Job: test (skipped) ===") {
		t.Errorf("expected test to be skipped, got:\n%s", out)
	}
	if !strings.Contains(out, "=== Job: deploy (skipped) ===") {
		t.Errorf("expected deploy to be skipped, got:\n%s", out)
	}
}
