package workflow

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
var validInputNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type Input struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type Step struct {
	Id   string            `yaml:"id"`
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

type Job struct {
	Needs []string          `yaml:"-"`
	Steps []Step            `yaml:"steps"`
	Env   map[string]string `yaml:"env"`
}

type Workflow struct {
	Name     string            `yaml:"name"`
	Quiet    bool              `yaml:"quiet"`
	Env      map[string]string `yaml:"-"`
	Inputs   map[string]Input  `yaml:"-"`
	Jobs     map[string]Job    `yaml:"-"`
	JobOrder []string          `yaml:"-"`
}

func (w *Workflow) UnmarshalYAML(value *yaml.Node) error {
	// Decode name field
	for i := 0; i < len(value.Content)-1; i += 2 {
		key := value.Content[i]
		val := value.Content[i+1]

		switch key.Value {
		case "name":
			w.Name = val.Value
		case "quiet":
			w.Quiet = val.Value == "true"
		case "env":
			w.Env = make(map[string]string)
			if err := val.Decode(&w.Env); err != nil {
				return fmt.Errorf("decoding workflow env: %w", err)
			}
		case "inputs":
			w.Inputs = make(map[string]Input)
			if err := val.Decode(&w.Inputs); err != nil {
				return fmt.Errorf("decoding workflow inputs: %w", err)
			}
		case "jobs":
			w.Jobs = make(map[string]Job)
			if val.Kind != yaml.MappingNode {
				return fmt.Errorf("jobs must be a mapping")
			}
			for j := 0; j < len(val.Content)-1; j += 2 {
				jobKey := val.Content[j]
				jobVal := val.Content[j+1]

				var job Job
				if err := jobVal.Decode(&job); err != nil {
					return fmt.Errorf("decoding job %q: %w", jobKey.Value, err)
				}

				// Parse "needs" field from raw YAML node
				if jobVal.Kind == yaml.MappingNode {
					for k := 0; k < len(jobVal.Content)-1; k += 2 {
						if jobVal.Content[k].Value == "needs" {
							needsNode := jobVal.Content[k+1]
							switch needsNode.Kind {
							case yaml.ScalarNode:
								job.Needs = []string{needsNode.Value}
							case yaml.SequenceNode:
								var needs []string
								if err := needsNode.Decode(&needs); err != nil {
									return fmt.Errorf("decoding needs for job %q: %w", jobKey.Value, err)
								}
								job.Needs = needs
							default:
								return fmt.Errorf("job %q: needs must be a string or list of strings", jobKey.Value)
							}
							break
						}
					}
				}

				w.JobOrder = append(w.JobOrder, jobKey.Value)
				w.Jobs[jobKey.Value] = job
			}
		}
	}

	return nil
}

func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow name is required")
	}
	for name := range w.Inputs {
		if !validInputNamePattern.MatchString(name) {
			return fmt.Errorf("input %q has invalid name: must contain only alphanumeric characters, hyphens, and underscores", name)
		}
	}
	if len(w.Jobs) == 0 {
		return fmt.Errorf("workflow must have at least one job")
	}
	for jobName, job := range w.Jobs {
		if len(job.Steps) == 0 {
			return fmt.Errorf("job %q must have at least one step", jobName)
		}
		seenIDs := make(map[string]bool)
		for i, step := range job.Steps {
			if step.Run == "" {
				return fmt.Errorf("step %d in job %q must have a run command", i+1, jobName)
			}
			if step.Id != "" {
				if !validIDPattern.MatchString(step.Id) {
					return fmt.Errorf("step %d in job %q has invalid id %q: must contain only alphanumeric characters and hyphens", i+1, jobName, step.Id)
				}
				if seenIDs[step.Id] {
					return fmt.Errorf("step %d in job %q has duplicate id %q", i+1, jobName, step.Id)
				}
				seenIDs[step.Id] = true
			}
		}
		for _, need := range job.Needs {
			if need == jobName {
				return fmt.Errorf("job %q cannot depend on itself", jobName)
			}
			if _, ok := w.Jobs[need]; !ok {
				return fmt.Errorf("job %q depends on unknown job %q", jobName, need)
			}
		}
	}

	order, err := w.ResolveOrder()
	if err != nil {
		return err
	}
	w.JobOrder = order
	return nil
}

// ResolveOrder returns a topological ordering of jobs using Kahn's algorithm.
// Jobs at the same dependency level preserve their YAML declaration order.
func (w *Workflow) ResolveOrder() ([]string, error) {
	// Build in-degree map and adjacency list
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // dependency -> jobs that depend on it
	for _, name := range w.JobOrder {
		inDegree[name] = 0
	}
	for _, name := range w.JobOrder {
		job := w.Jobs[name]
		inDegree[name] = len(job.Needs)
		for _, need := range job.Needs {
			dependents[need] = append(dependents[need], name)
		}
	}

	// Seed queue with jobs that have no dependencies (in YAML order)
	var queue []string
	for _, name := range w.JobOrder {
		if inDegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		// Collect newly unblocked jobs, preserving YAML order
		var unblocked []string
		for _, dep := range dependents[current] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				unblocked = append(unblocked, dep)
			}
		}
		// Insert unblocked jobs maintaining their relative YAML order
		if len(unblocked) > 0 {
			yamlPos := make(map[string]int)
			for i, name := range w.JobOrder {
				yamlPos[name] = i
			}
			// Sort unblocked by YAML position using insertion sort (small slice)
			for i := 1; i < len(unblocked); i++ {
				for j := i; j > 0 && yamlPos[unblocked[j]] < yamlPos[unblocked[j-1]]; j-- {
					unblocked[j], unblocked[j-1] = unblocked[j-1], unblocked[j]
				}
			}
			queue = append(queue, unblocked...)
		}
	}

	if len(sorted) != len(w.Jobs) {
		return nil, fmt.Errorf("circular dependency detected among jobs")
	}
	return sorted, nil
}
