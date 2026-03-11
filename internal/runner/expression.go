package runner

import "regexp"

var stepsExpressionPattern = regexp.MustCompile(`\$\{\{\s*steps\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)
var inputsExpressionPattern = regexp.MustCompile(`\$\{\{\s*inputs\.([a-zA-Z0-9_-]+)\s*\}\}`)
var needsExpressionPattern = regexp.MustCompile(`\$\{\{\s*needs\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)

func expandExpressions(command string, stepOutputs map[string]map[string]string, inputs map[string]string, jobOutputs map[string]map[string]string) string {
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

	return result
}
