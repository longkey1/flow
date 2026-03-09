package runner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/longkey1/flow/internal/workflow"
)

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

	if err := r.Run(wf); err != nil {
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

	if err := r.Run(wf); err != nil {
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

	err := r.Run(wf)
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
		"build":      {Steps: []workflow.Step{{Run: "exit 1"}}},
		"independent": {Steps: []workflow.Step{{Run: "echo independent"}}},
		"test":       {Needs: []string{"build"}, Steps: []workflow.Step{{Run: "echo test"}}},
	}, []string{"build", "independent", "test"})

	err := r.Run(wf)
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

	if err := r.Run(wf); err != nil {
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

	if err := r.Run(wf); err != nil {
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

	if err := r.Run(wf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "[]") {
		t.Errorf("expected '[]' in output, got:\n%s", stdout.String())
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

	err := r.Run(wf)
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
