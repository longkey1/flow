package runner

import (
	"testing"

	"github.com/longkey1/flow/internal/workflow"
)

func TestExpandExpressions(t *testing.T) {
	outputs := map[string]map[string]string{
		"get-version": {"version": "1.2.3"},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic substitution",
			input:    `echo "${{ steps.get-version.outputs.version }}"`,
			expected: `echo "1.2.3"`,
		},
		{
			name:     "no spaces in braces",
			input:    `echo "${{steps.get-version.outputs.version}}"`,
			expected: `echo "1.2.3"`,
		},
		{
			name:     "multiple substitutions",
			input:    `echo "${{ steps.get-version.outputs.version }} ${{ steps.get-version.outputs.version }}"`,
			expected: `echo "1.2.3 1.2.3"`,
		},
		{
			name:     "unknown step returns empty",
			input:    `echo "${{ steps.unknown.outputs.key }}"`,
			expected: `echo ""`,
		},
		{
			name:     "unknown key returns empty",
			input:    `echo "${{ steps.get-version.outputs.unknown }}"`,
			expected: `echo ""`,
		},
		{
			name:     "no expression unchanged",
			input:    `echo "hello world"`,
			expected: `echo "hello world"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandExpressions(tt.input, outputs, nil, nil, nil)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestExpandExpressionsInputs(t *testing.T) {
	inputs := map[string]string{
		"name":     "World",
		"greeting": "Hi",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic input substitution",
			input:    `echo "${{ inputs.name }}"`,
			expected: `echo "World"`,
		},
		{
			name:     "no spaces in braces",
			input:    `echo "${{inputs.greeting}}"`,
			expected: `echo "Hi"`,
		},
		{
			name:     "multiple inputs",
			input:    `echo "${{ inputs.greeting }}, ${{ inputs.name }}!"`,
			expected: `echo "Hi, World!"`,
		},
		{
			name:     "unknown input returns empty",
			input:    `echo "${{ inputs.unknown }}"`,
			expected: `echo ""`,
		},
		{
			name:     "mixed steps and inputs",
			input:    `echo "${{ steps.s1.outputs.key }} ${{ inputs.name }}"`,
			expected: `echo "val World"`,
		},
	}

	stepOutputs := map[string]map[string]string{
		"s1": {"key": "val"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandExpressions(tt.input, stepOutputs, inputs, nil, nil)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestExpandExpressionsNeeds(t *testing.T) {
	jobOutputs := map[string]map[string]string{
		"build": {"version": "1.0.0", "artifact": "app.tar.gz"},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic needs substitution",
			input:    `echo "${{ needs.build.outputs.version }}"`,
			expected: `echo "1.0.0"`,
		},
		{
			name:     "no spaces in braces",
			input:    `echo "${{needs.build.outputs.artifact}}"`,
			expected: `echo "app.tar.gz"`,
		},
		{
			name:     "multiple needs substitutions",
			input:    `echo "${{ needs.build.outputs.version }} ${{ needs.build.outputs.artifact }}"`,
			expected: `echo "1.0.0 app.tar.gz"`,
		},
		{
			name:     "unknown job returns empty",
			input:    `echo "${{ needs.unknown.outputs.key }}"`,
			expected: `echo ""`,
		},
		{
			name:     "unknown key returns empty",
			input:    `echo "${{ needs.build.outputs.unknown }}"`,
			expected: `echo ""`,
		},
		{
			name:     "mixed steps inputs and needs",
			input:    `echo "${{ steps.s1.outputs.key }} ${{ inputs.name }} ${{ needs.build.outputs.version }}"`,
			expected: `echo "val World 1.0.0"`,
		},
	}

	stepOutputs := map[string]map[string]string{
		"s1": {"key": "val"},
	}
	inputs := map[string]string{
		"name": "World",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandExpressions(tt.input, stepOutputs, inputs, jobOutputs, nil)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestExpandWorkflowOutputs(t *testing.T) {
	jobOutputs := map[string]map[string]string{
		"build": {"version": "1.0.0", "status": "ok"},
		"test":  {"result": "passed"},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic jobs substitution",
			input:    `${{ jobs.build.outputs.version }}`,
			expected: `1.0.0`,
		},
		{
			name:     "no spaces in braces",
			input:    `${{jobs.build.outputs.status}}`,
			expected: `ok`,
		},
		{
			name:     "multiple jobs substitutions",
			input:    `${{ jobs.build.outputs.version }} ${{ jobs.test.outputs.result }}`,
			expected: `1.0.0 passed`,
		},
		{
			name:     "unknown job returns empty",
			input:    `${{ jobs.unknown.outputs.key }}`,
			expected: ``,
		},
		{
			name:     "unknown key returns empty",
			input:    `${{ jobs.build.outputs.unknown }}`,
			expected: ``,
		},
		{
			name:     "no expression unchanged",
			input:    `hello world`,
			expected: `hello world`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandWorkflowOutputs(tt.input, jobOutputs)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestExpandExpressionsMatrix(t *testing.T) {
	matrixValues := map[string]string{
		"node":   "18",
		"target": "api",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic matrix substitution",
			input:    `echo "${{ matrix.node }}"`,
			expected: `echo "18"`,
		},
		{
			name:     "multiple matrix values",
			input:    `echo "${{ matrix.node }} ${{ matrix.target }}"`,
			expected: `echo "18 api"`,
		},
		{
			name:     "unknown matrix key returns empty",
			input:    `echo "${{ matrix.unknown }}"`,
			expected: `echo ""`,
		},
		{
			name:     "nil matrix returns empty",
			input:    `echo "${{ matrix.key }}"`,
			expected: `echo ""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := matrixValues
			if tt.name == "nil matrix returns empty" {
				mv = nil
			}
			got := expandExpressions(tt.input, nil, nil, nil, mv)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestResolveMatrixParamStatic(t *testing.T) {
	param := workflow.MatrixParam{Values: []string{"a", "b", "c"}}
	values, err := resolveMatrixParam(param, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 3 || values[0] != "a" || values[1] != "b" || values[2] != "c" {
		t.Errorf("expected [a, b, c], got %v", values)
	}
}

func TestResolveMatrixParamFromJson(t *testing.T) {
	param := workflow.MatrixParam{Expression: `${{ fromJson(needs.setup.outputs.targets) }}`}
	jobOutputs := map[string]map[string]string{
		"setup": {"targets": `["api","web","worker"]`},
	}
	values, err := resolveMatrixParam(param, nil, jobOutputs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 3 || values[0] != "api" || values[1] != "web" || values[2] != "worker" {
		t.Errorf("expected [api, web, worker], got %v", values)
	}
}

func TestResolveMatrixParamFromJsonInvalidJSON(t *testing.T) {
	param := workflow.MatrixParam{Expression: `${{ fromJson(needs.setup.outputs.targets) }}`}
	jobOutputs := map[string]map[string]string{
		"setup": {"targets": `not-json`},
	}
	_, err := resolveMatrixParam(param, nil, jobOutputs)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCartesianProductSingleKey(t *testing.T) {
	params := map[string][]string{
		"node": {"16", "18"},
	}
	result := cartesianProduct(params)
	if len(result) != 2 {
		t.Fatalf("expected 2 combinations, got %d", len(result))
	}
	if result[0]["node"] != "16" || result[1]["node"] != "18" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestCartesianProductMultipleKeys(t *testing.T) {
	params := map[string][]string{
		"node": {"16", "18"},
		"os":   {"linux", "darwin"},
	}
	result := cartesianProduct(params)
	if len(result) != 4 {
		t.Fatalf("expected 4 combinations, got %d", len(result))
	}
	// Sorted keys: node, os
	expected := []map[string]string{
		{"node": "16", "os": "linux"},
		{"node": "16", "os": "darwin"},
		{"node": "18", "os": "linux"},
		{"node": "18", "os": "darwin"},
	}
	for i, combo := range result {
		if combo["node"] != expected[i]["node"] || combo["os"] != expected[i]["os"] {
			t.Errorf("combination %d: expected %v, got %v", i, expected[i], combo)
		}
	}
}
