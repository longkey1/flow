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
			got := expandExpressions(tt.input, outputs)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
