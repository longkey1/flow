package runner

import "testing"

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
			got := expandExpressions(tt.input, outputs, nil, nil)
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
			got := expandExpressions(tt.input, stepOutputs, inputs, nil)
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
			got := expandExpressions(tt.input, stepOutputs, inputs, jobOutputs)
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
