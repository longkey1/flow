package runner

import "regexp"

var expressionPattern = regexp.MustCompile(`\$\{\{\s*steps\.([a-zA-Z0-9-]+)\.outputs\.([a-zA-Z0-9_-]+)\s*\}\}`)

func expandExpressions(command string, stepOutputs map[string]map[string]string) string {
	return expressionPattern.ReplaceAllStringFunc(command, func(match string) string {
		parts := expressionPattern.FindStringSubmatch(match)
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
}
