package action

import (
	"fmt"
	"regexp"
)

var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
var validInputNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Input struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type Output struct {
	Description string `yaml:"description"`
}

type Step struct {
	Id   string            `yaml:"id"`
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

type Runs struct {
	Steps []Step `yaml:"steps"`
}

type Action struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Inputs      map[string]Input  `yaml:"inputs"`
	Outputs     map[string]Output `yaml:"outputs"`
	Runs        Runs              `yaml:"runs"`
}

func (a *Action) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("action name is required")
	}
	for name := range a.Inputs {
		if !validInputNamePattern.MatchString(name) {
			return fmt.Errorf("input %q has invalid name: must contain only alphanumeric characters, hyphens, and underscores", name)
		}
	}
	if len(a.Runs.Steps) == 0 {
		return fmt.Errorf("action must have at least one step")
	}
	seenIDs := make(map[string]bool)
	for i, step := range a.Runs.Steps {
		if step.Run == "" {
			return fmt.Errorf("step %d must have a run command", i+1)
		}
		if step.Id != "" {
			if !validIDPattern.MatchString(step.Id) {
				return fmt.Errorf("step %d has invalid id %q: must contain only alphanumeric characters and hyphens", i+1, step.Id)
			}
			if seenIDs[step.Id] {
				return fmt.Errorf("step %d has duplicate id %q", i+1, step.Id)
			}
			seenIDs[step.Id] = true
		}
	}
	return nil
}
