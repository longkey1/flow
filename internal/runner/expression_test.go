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
			got := expandExpressions(tt.input, outputs, nil)
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
			got := expandExpressions(tt.input, stepOutputs, inputs)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
