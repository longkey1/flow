package runner

import (
	"testing"
)

func TestConditionSuccess(t *testing.T) {
	result, err := evaluateCondition("success()", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected success() to be true when not failed")
	}
}

func TestConditionSuccessWhenFailed(t *testing.T) {
	result, err := evaluateCondition("success()", true, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result {
		t.Error("expected success() to be false when failed")
	}
}

func TestConditionFailure(t *testing.T) {
	result, err := evaluateCondition("failure()", true, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected failure() to be true when failed")
	}
}

func TestConditionFailureWhenNotFailed(t *testing.T) {
	result, err := evaluateCondition("failure()", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result {
		t.Error("expected failure() to be false when not failed")
	}
}

func TestConditionAlways(t *testing.T) {
	for _, failed := range []bool{true, false} {
		result, err := evaluateCondition("always()", failed, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !result {
			t.Errorf("expected always() to be true when failed=%v", failed)
		}
	}
}

func TestConditionEqualTrue(t *testing.T) {
	result, err := evaluateCondition("'hello' == 'hello'", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected 'hello' == 'hello' to be true")
	}
}

func TestConditionEqualFalse(t *testing.T) {
	result, err := evaluateCondition("'hello' == 'world'", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result {
		t.Error("expected 'hello' == 'world' to be false")
	}
}

func TestConditionNotEqual(t *testing.T) {
	result, err := evaluateCondition("'hello' != 'world'", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected 'hello' != 'world' to be true")
	}
}

func TestConditionAnd(t *testing.T) {
	result, err := evaluateCondition("success() && 'a' == 'a'", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected true && true to be true")
	}
}

func TestConditionAndFalse(t *testing.T) {
	result, err := evaluateCondition("success() && failure()", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result {
		t.Error("expected true && false to be false")
	}
}

func TestConditionOr(t *testing.T) {
	result, err := evaluateCondition("failure() || success()", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected false || true to be true")
	}
}

func TestConditionNot(t *testing.T) {
	result, err := evaluateCondition("!failure()", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected !false to be true")
	}
}

func TestConditionParentheses(t *testing.T) {
	result, err := evaluateCondition("!(failure() && success())", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected !(false && true) to be true")
	}
}

func TestConditionTruthyValues(t *testing.T) {
	tests := []struct {
		condition string
		expected  bool
	}{
		{"''", false},           // empty string is falsy
		{"'false'", false},      // "false" is falsy
		{"'0'", false},          // "0" is falsy
		{"'hello'", true},       // non-empty string is truthy
		{"'true'", true},        // "true" is truthy
		{"'1'", true},           // "1" is truthy
	}

	for _, tt := range tests {
		result, err := evaluateCondition(tt.condition, false, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("condition %q: %v", tt.condition, err)
		}
		if result != tt.expected {
			t.Errorf("condition %q: expected %v, got %v", tt.condition, tt.expected, result)
		}
	}
}

func TestConditionWithVariableExpansion(t *testing.T) {
	inputs := map[string]string{"env": "prod"}
	result, err := evaluateCondition("${{ inputs.env }} == 'prod'", false, nil, inputs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected expanded variable to match 'prod'")
	}
}

func TestConditionComplex(t *testing.T) {
	inputs := map[string]string{"force": "true"}
	result, err := evaluateCondition("failure() || ${{ inputs.force }} == 'true'", false, nil, inputs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected failure() || true to be true")
	}
}

func TestConditionUnknownFunction(t *testing.T) {
	_, err := evaluateCondition("unknown()", false, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for unknown function")
	}
}

func TestPreprocessCondition(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"${{ inputs.env != 'prod' }}", "inputs.env != 'prod'"},
		{"${{ needs.setup.outputs.skip != 'true' }}", "needs.setup.outputs.skip != 'true'"},
		{"${{ success() && inputs.deploy == 'true' }}", "success() && inputs.deploy == 'true'"},
		{"${{ inputs.env }} == 'prod'", "${{ inputs.env }} == 'prod'"}, // partial - not stripped
		{"success()", "success()"},                                     // no wrapper
		{"  ${{  inputs.x  }}  ", "inputs.x"},                          // extra whitespace
	}
	for _, tt := range tests {
		got := preprocessCondition(tt.input)
		if got != tt.expected {
			t.Errorf("preprocessCondition(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConditionWrappedExpression(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		inputs    map[string]string
		jobOut    map[string]map[string]string
		stepOut   map[string]map[string]string
		expected  bool
	}{
		{
			name:      "inputs != in wrapped expression",
			condition: "${{ inputs.env != 'prod' }}",
			inputs:    map[string]string{"env": "staging"},
			expected:  true,
		},
		{
			name:      "inputs == in wrapped expression (false)",
			condition: "${{ inputs.env != 'prod' }}",
			inputs:    map[string]string{"env": "prod"},
			expected:  false,
		},
		{
			name:      "needs reference in wrapped expression",
			condition: "${{ needs.setup.outputs.skip != 'true' }}",
			jobOut:    map[string]map[string]string{"setup": {"skip": "false"}},
			expected:  true,
		},
		{
			name:      "needs reference skip=true",
			condition: "${{ needs.setup.outputs.skip != 'true' }}",
			jobOut:    map[string]map[string]string{"setup": {"skip": "true"}},
			expected:  false,
		},
		{
			name:      "success() && inputs in wrapped expression",
			condition: "${{ success() && inputs.deploy == 'true' }}",
			inputs:    map[string]string{"deploy": "true"},
			expected:  true,
		},
		{
			name:      "success() && inputs false",
			condition: "${{ success() && inputs.deploy == 'true' }}",
			inputs:    map[string]string{"deploy": "false"},
			expected:  false,
		},
		{
			name:      "steps reference in wrapped expression",
			condition: "${{ steps.check.outputs.result == 'ok' }}",
			stepOut:   map[string]map[string]string{"check": {"result": "ok"}},
			expected:  true,
		},
		{
			name:      "backward compat: partial expression",
			condition: "${{ inputs.env }} == 'prod'",
			inputs:    map[string]string{"env": "prod"},
			expected:  true,
		},
		{
			name:      "backward compat: failure with partial expression",
			condition: "failure() || ${{ inputs.force }} == 'true'",
			inputs:    map[string]string{"force": "true"},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateCondition(tt.condition, false, tt.stepOut, tt.inputs, tt.jobOut, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("evaluateCondition(%q) = %v, want %v", tt.condition, result, tt.expected)
			}
		})
	}
}

func TestConditionBoolLiterals(t *testing.T) {
	result, err := evaluateCondition("true", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result {
		t.Error("expected true literal to be truthy")
	}

	result, err = evaluateCondition("false", false, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result {
		t.Error("expected false literal to be falsy")
	}
}
