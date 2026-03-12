package runner

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/longkey1/flow/internal/workflow"
)

var stepsExpressionPattern = regexp.MustCompile(`\$\{\{\s*steps\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)
var inputsExpressionPattern = regexp.MustCompile(`\$\{\{\s*inputs\.([a-zA-Z0-9_-]+)\s*\}\}`)
var needsExpressionPattern = regexp.MustCompile(`\$\{\{\s*needs\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)
var matrixExpressionPattern = regexp.MustCompile(`\$\{\{\s*matrix\.([a-zA-Z0-9_-]+)\s*\}\}`)
var fromJsonPattern = regexp.MustCompile(`^\$\{\{\s*fromJson\(\s*(.*?)\s*\)\s*\}\}$`)

func expandExpressions(command string, stepOutputs map[string]map[string]string, inputs map[string]string, jobOutputs map[string]map[string]string, matrixValues map[string]string) string {
	result := stepsExpressionPattern.ReplaceAllStringFunc(command, func(match string) string {
		parts := stepsExpressionPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		stepID := parts[1]
		key := parts[2]
		if outputs, ok := stepOutputs[stepID]; ok {
			if val, ok := outputs[key]; ok {
				return val
			}
		}
		return ""
	})

	result = inputsExpressionPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := inputsExpressionPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		key := parts[1]
		if val, ok := inputs[key]; ok {
			return val
		}
		return ""
	})

	result = needsExpressionPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := needsExpressionPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		jobName := parts[1]
		key := parts[2]
		if outputs, ok := jobOutputs[jobName]; ok {
			if val, ok := outputs[key]; ok {
				return val
			}
		}
		return ""
	})

	result = matrixExpressionPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := matrixExpressionPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		key := parts[1]
		if matrixValues != nil {
			if val, ok := matrixValues[key]; ok {
				return val
			}
		}
		return ""
	})

	return result
}

var jobsExpressionPattern = regexp.MustCompile(`\$\{\{\s*jobs\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)

func resolveMatrixParam(param workflow.MatrixParam, inputs map[string]string, jobOutputs map[string]map[string]string) ([]string, error) {
	if len(param.Values) > 0 {
		return param.Values, nil
	}

	expr := param.Expression
	matches := fromJsonPattern.FindStringSubmatch(expr)
	if matches == nil {
		return nil, fmt.Errorf("matrix expression must use fromJson(): %s", expr)
	}

	innerExpr := matches[1]
	// Wrap inner expression in ${{ }} and expand
	wrapped := "${{ " + innerExpr + " }}"
	resolved := expandExpressions(wrapped, nil, inputs, jobOutputs, nil)

	var values []string
	if err := json.Unmarshal([]byte(resolved), &values); err != nil {
		return nil, fmt.Errorf("fromJson: failed to parse %q as string array: %w", resolved, err)
	}
	return values, nil
}

func cartesianProduct(params map[string][]string) []map[string]string {
	// Sort keys for deterministic order
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := []map[string]string{{}}
	for _, key := range keys {
		values := params[key]
		var newResult []map[string]string
		for _, combo := range result {
			for _, val := range values {
				newCombo := make(map[string]string, len(combo)+1)
				for k, v := range combo {
					newCombo[k] = v
				}
				newCombo[key] = val
				newResult = append(newResult, newCombo)
			}
		}
		result = newResult
	}
	return result
}

// expandVariableRef resolves a bare variable reference (without ${{ }}) to its value.
// Supports: inputs.X, steps.ID.outputs.KEY, needs.JOB.outputs.KEY, matrix.KEY.
// Returns (resolved value, true) if matched, or (ref, false) if not a known variable pattern.
func expandVariableRef(ref string, stepOutputs map[string]map[string]string, inputs map[string]string, jobOutputs map[string]map[string]string, matrixValues map[string]string) (string, bool) {
	parts := strings.Split(ref, ".")

	if len(parts) == 2 && parts[0] == "inputs" {
		if inputs != nil {
			if val, ok := inputs[parts[1]]; ok {
				return val, true
			}
		}
		return "", true
	}

	if len(parts) == 2 && parts[0] == "matrix" {
		if matrixValues != nil {
			if val, ok := matrixValues[parts[1]]; ok {
				return val, true
			}
		}
		return "", true
	}

	if len(parts) == 4 && parts[0] == "steps" && parts[2] == "outputs" {
		if stepOutputs != nil {
			if outputs, ok := stepOutputs[parts[1]]; ok {
				if val, ok := outputs[parts[3]]; ok {
					return val, true
				}
			}
		}
		return "", true
	}

	if len(parts) == 4 && parts[0] == "needs" && parts[2] == "outputs" {
		if jobOutputs != nil {
			if outputs, ok := jobOutputs[parts[1]]; ok {
				if val, ok := outputs[parts[3]]; ok {
					return val, true
				}
			}
		}
		return "", true
	}

	return ref, false
}

func expandWorkflowOutputs(expr string, jobOutputs map[string]map[string]string) string {
	return jobsExpressionPattern.ReplaceAllStringFunc(expr, func(match string) string {
		parts := jobsExpressionPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		jobName := parts[1]
		key := parts[2]
		if outputs, ok := jobOutputs[jobName]; ok {
			if val, ok := outputs[key]; ok {
				return val
			}
		}
		return ""
	})
}
