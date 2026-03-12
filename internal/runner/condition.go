package runner

import (
	"fmt"
	"strings"
)

// evaluateCondition evaluates an if condition string and returns whether the step/job should run.
// The condition is first expanded using expandExpressions, then parsed and evaluated.
// Supported: success(), failure(), always(), ==, !=, &&, ||, !, parentheses, string literals.
func evaluateCondition(condition string, jobFailed bool, stepOutputs map[string]map[string]string, inputs map[string]string, jobOutputs map[string]map[string]string, matrixValues map[string]string) (bool, error) {
	// Expand ${{ }} expressions in the condition
	expanded := expandExpressions(condition, stepOutputs, inputs, jobOutputs, matrixValues)

	p := &condParser{input: expanded, pos: 0, jobFailed: jobFailed}
	result, err := p.parseOr()
	if err != nil {
		return false, fmt.Errorf("evaluating condition %q: %w", condition, err)
	}
	p.skipSpaces()
	if p.pos < len(p.input) {
		return false, fmt.Errorf("evaluating condition %q: unexpected character at position %d", condition, p.pos)
	}
	return isTruthy(result, jobFailed), nil
}

// isTruthy determines if a parsed value is truthy.
// Special status values "success", "failure", "always" are handled by the parser.
// For string values: empty string, "false", "0" are falsy; everything else is truthy.
func isTruthy(val condValue, jobFailed bool) bool {
	switch val.kind {
	case condBool:
		return val.boolVal
	case condString:
		return val.strVal != "" && val.strVal != "false" && val.strVal != "0"
	default:
		return false
	}
}

type condKind int

const (
	condBool   condKind = iota
	condString condKind = iota
)

type condValue struct {
	kind    condKind
	boolVal bool
	strVal  string
}

func boolValue(b bool) condValue {
	return condValue{kind: condBool, boolVal: b}
}

func stringValue(s string) condValue {
	return condValue{kind: condString, strVal: s}
}

type condParser struct {
	input     string
	pos       int
	jobFailed bool
}

func (p *condParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *condParser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *condParser) parseOr() (condValue, error) {
	left, err := p.parseAnd()
	if err != nil {
		return condValue{}, err
	}

	for {
		p.skipSpaces()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '|' && p.input[p.pos+1] == '|' {
			p.pos += 2
			right, err := p.parseAnd()
			if err != nil {
				return condValue{}, err
			}
			leftBool := isTruthy(left, p.jobFailed)
			rightBool := isTruthy(right, p.jobFailed)
			left = boolValue(leftBool || rightBool)
		} else {
			break
		}
	}
	return left, nil
}

func (p *condParser) parseAnd() (condValue, error) {
	left, err := p.parseComparison()
	if err != nil {
		return condValue{}, err
	}

	for {
		p.skipSpaces()
		if p.pos+1 < len(p.input) && p.input[p.pos] == '&' && p.input[p.pos+1] == '&' {
			p.pos += 2
			right, err := p.parseComparison()
			if err != nil {
				return condValue{}, err
			}
			leftBool := isTruthy(left, p.jobFailed)
			rightBool := isTruthy(right, p.jobFailed)
			left = boolValue(leftBool && rightBool)
		} else {
			break
		}
	}
	return left, nil
}

func (p *condParser) parseComparison() (condValue, error) {
	left, err := p.parseUnary()
	if err != nil {
		return condValue{}, err
	}

	p.skipSpaces()
	if p.pos+1 < len(p.input) {
		op := ""
		if p.input[p.pos] == '=' && p.input[p.pos+1] == '=' {
			op = "=="
		} else if p.input[p.pos] == '!' && p.input[p.pos+1] == '=' {
			op = "!="
		}
		if op != "" {
			p.pos += 2
			right, err := p.parseUnary()
			if err != nil {
				return condValue{}, err
			}
			leftStr := valueToString(left)
			rightStr := valueToString(right)
			if op == "==" {
				return boolValue(leftStr == rightStr), nil
			}
			return boolValue(leftStr != rightStr), nil
		}
	}

	return left, nil
}

func (p *condParser) parseUnary() (condValue, error) {
	p.skipSpaces()
	if p.pos < len(p.input) && p.input[p.pos] == '!' {
		p.pos++
		val, err := p.parseUnary()
		if err != nil {
			return condValue{}, err
		}
		return boolValue(!isTruthy(val, p.jobFailed)), nil
	}
	return p.parsePrimary()
}

func (p *condParser) parsePrimary() (condValue, error) {
	p.skipSpaces()

	if p.pos >= len(p.input) {
		return stringValue(""), nil
	}

	// Parenthesized expression
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseOr()
		if err != nil {
			return condValue{}, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return condValue{}, fmt.Errorf("expected closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	// String literal with single quotes
	if p.input[p.pos] == '\'' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != '\'' {
			p.pos++
		}
		if p.pos >= len(p.input) {
			return condValue{}, fmt.Errorf("unterminated string literal")
		}
		val := p.input[start:p.pos]
		p.pos++ // skip closing quote
		return stringValue(val), nil
	}

	// Function call or bare word
	start := p.pos
	for p.pos < len(p.input) && isWordChar(p.input[p.pos]) {
		p.pos++
	}
	word := p.input[start:p.pos]

	p.skipSpaces()
	// Check for function call
	if p.pos < len(p.input) && p.input[p.pos] == '(' {
		p.pos++
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return condValue{}, fmt.Errorf("expected closing parenthesis for function %s()", word)
		}
		p.pos++
		return p.evalFunction(word)
	}

	// Bare word (expanded expression result or boolean literal)
	if word == "true" {
		return boolValue(true), nil
	}
	if word == "false" {
		return boolValue(false), nil
	}
	return stringValue(word), nil
}

func (p *condParser) evalFunction(name string) (condValue, error) {
	switch name {
	case "success":
		return boolValue(!p.jobFailed), nil
	case "failure":
		return boolValue(p.jobFailed), nil
	case "always":
		return boolValue(true), nil
	default:
		return condValue{}, fmt.Errorf("unknown function: %s()", name)
	}
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '_' || c == '-' || c == '.'
}

func valueToString(v condValue) string {
	switch v.kind {
	case condBool:
		if v.boolVal {
			return "true"
		}
		return "false"
	case condString:
		return v.strVal
	default:
		return ""
	}
}

// hasAlwaysCondition checks if a condition contains always() function.
func hasAlwaysCondition(condition string) bool {
	return strings.Contains(condition, "always()")
}

// hasFailureCondition checks if a condition contains failure() function.
func hasFailureCondition(condition string) bool {
	return strings.Contains(condition, "failure()")
}
