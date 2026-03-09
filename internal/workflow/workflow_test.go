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
