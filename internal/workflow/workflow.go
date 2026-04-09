package workflow

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
var validInputNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var validShells = map[string]bool{"": true, "sh": true, "bash": true}

type Input struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type Step struct {
	Id    string            `yaml:"id"`
	Name  string            `yaml:"name"`
	If    string            `yaml:"if"`
	Run   string            `yaml:"run"`
	Shell string            `yaml:"shell"`
	Uses  string            `yaml:"uses"`
	With  map[string]string `yaml:"with"`
	Env   map[string]string `yaml:"env"`
}

type RunDefaults struct {
	Shell string `yaml:"shell"`
}

type Defaults struct {
	Run RunDefaults `yaml:"run"`
}

type MatrixParam struct {
	Values     []string // static values from YAML list
	Expression string   // dynamic expression (e.g. fromJson(...))
}

type Strategy struct {
	Matrix      map[string]MatrixParam
	MaxParallel int
}

type Job struct {
	Needs    []string          `yaml:"-"`
	If       string            `yaml:"-"`
	Outputs  map[string]string `yaml:"-"`
	Uses     string            `yaml:"-"`
	With     map[string]string `yaml:"-"`
	Strategy *Strategy         `yaml:"-"`
	Defaults *Defaults         `yaml:"-"`
	Steps    []Step            `yaml:"steps"`
	Env      map[string]string `yaml:"env"`
}

type Workflow struct {
	Name     string            `yaml:"name"`
	Quiet    bool              `yaml:"quiet"`
	Env      map[string]string `yaml:"-"`
	Inputs   map[string]Input  `yaml:"-"`
	Outputs  map[string]string `yaml:"-"`
	Defaults *Defaults         `yaml:"-"`
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
		case "outputs":
			w.Outputs = make(map[string]string)
			if err := val.Decode(&w.Outputs); err != nil {
				return fmt.Errorf("decoding workflow outputs: %w", err)
			}
		case "defaults":
			var defaults Defaults
			if err := val.Decode(&defaults); err != nil {
				return fmt.Errorf("decoding workflow defaults: %w", err)
			}
			w.Defaults = &defaults
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

				// Parse "needs" and "outputs" fields from raw YAML node
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
						}
						if jobVal.Content[k].Value == "outputs" {
							outputsNode := jobVal.Content[k+1]
							if outputsNode.Kind != yaml.MappingNode {
								return fmt.Errorf("job %q: outputs must be a mapping", jobKey.Value)
							}
							outputs := make(map[string]string)
							if err := outputsNode.Decode(&outputs); err != nil {
								return fmt.Errorf("decoding outputs for job %q: %w", jobKey.Value, err)
							}
							job.Outputs = outputs
						}
						if jobVal.Content[k].Value == "if" {
							job.If = jobVal.Content[k+1].Value
						}
						if jobVal.Content[k].Value == "uses" {
							job.Uses = jobVal.Content[k+1].Value
						}
						if jobVal.Content[k].Value == "with" {
							withNode := jobVal.Content[k+1]
							if withNode.Kind != yaml.MappingNode {
								return fmt.Errorf("job %q: with must be a mapping", jobKey.Value)
							}
							withMap := make(map[string]string)
							if err := withNode.Decode(&withMap); err != nil {
								return fmt.Errorf("decoding with for job %q: %w", jobKey.Value, err)
							}
							job.With = withMap
						}
						if jobVal.Content[k].Value == "defaults" {
							defaultsNode := jobVal.Content[k+1]
							var defaults Defaults
							if err := defaultsNode.Decode(&defaults); err != nil {
								return fmt.Errorf("decoding defaults for job %q: %w", jobKey.Value, err)
							}
							job.Defaults = &defaults
						}
						if jobVal.Content[k].Value == "strategy" {
							strategyNode := jobVal.Content[k+1]
							if strategyNode.Kind != yaml.MappingNode {
								return fmt.Errorf("job %q: strategy must be a mapping", jobKey.Value)
							}
							strategy := &Strategy{}
							for s := 0; s < len(strategyNode.Content)-1; s += 2 {
								if strategyNode.Content[s].Value == "matrix" {
									matrixNode := strategyNode.Content[s+1]
									if matrixNode.Kind != yaml.MappingNode {
										return fmt.Errorf("job %q: strategy.matrix must be a mapping", jobKey.Value)
									}
									matrix := make(map[string]MatrixParam)
									for m := 0; m < len(matrixNode.Content)-1; m += 2 {
										paramKey := matrixNode.Content[m].Value
										paramVal := matrixNode.Content[m+1]
										switch paramVal.Kind {
										case yaml.SequenceNode:
											var values []string
											if err := paramVal.Decode(&values); err != nil {
												return fmt.Errorf("decoding matrix param %q for job %q: %w", paramKey, jobKey.Value, err)
											}
											matrix[paramKey] = MatrixParam{Values: values}
										case yaml.ScalarNode:
											matrix[paramKey] = MatrixParam{Expression: paramVal.Value}
										default:
											return fmt.Errorf("job %q: matrix param %q must be a list or expression string", jobKey.Value, paramKey)
										}
									}
									strategy.Matrix = matrix
								}
								if strategyNode.Content[s].Value == "max-parallel" {
									var maxParallel int
									if err := strategyNode.Content[s+1].Decode(&maxParallel); err != nil {
										return fmt.Errorf("job %q: strategy.max-parallel must be a positive integer", jobKey.Value)
									}
									if maxParallel < 1 {
										return fmt.Errorf("job %q: strategy.max-parallel must be a positive integer", jobKey.Value)
									}
									strategy.MaxParallel = maxParallel
								}
							}
							job.Strategy = strategy
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
	if w.Defaults != nil {
		if !validShells[w.Defaults.Run.Shell] {
			return fmt.Errorf("workflow has invalid defaults.run.shell %q: must be sh or bash", w.Defaults.Run.Shell)
		}
	}
	if len(w.Jobs) == 0 {
		return fmt.Errorf("workflow must have at least one job")
	}
	for jobName, job := range w.Jobs {
		if job.Strategy != nil {
			if len(job.Strategy.Matrix) == 0 {
				return fmt.Errorf("job %q: strategy.matrix must have at least one key", jobName)
			}
			for key, param := range job.Strategy.Matrix {
				if param.Expression == "" && len(param.Values) == 0 {
					return fmt.Errorf("job %q: matrix param %q must have values or an expression", jobName, key)
				}
			}
		}
		if job.Defaults != nil {
			if !validShells[job.Defaults.Run.Shell] {
				return fmt.Errorf("job %q has invalid defaults.run.shell %q: must be sh or bash", jobName, job.Defaults.Run.Shell)
			}
		}
		if job.Uses != "" && len(job.Steps) > 0 {
			return fmt.Errorf("job %q cannot have both uses and steps", jobName)
		}
		if job.Uses == "" && len(job.With) > 0 && job.Strategy == nil {
			return fmt.Errorf("job %q has with but no uses", jobName)
		}
		if job.Uses != "" {
			// uses job: skip step validation
		} else {
			if len(job.Steps) == 0 {
				return fmt.Errorf("job %q must have at least one step", jobName)
			}
			seenIDs := make(map[string]bool)
			for i, step := range job.Steps {
				if step.Run == "" && step.Uses == "" {
					return fmt.Errorf("step %d in job %q must have a run command or uses reference", i+1, jobName)
				}
				if step.Run != "" && step.Uses != "" {
					return fmt.Errorf("step %d in job %q cannot have both run and uses", i+1, jobName)
				}
				if step.Uses == "" && len(step.With) > 0 {
					return fmt.Errorf("step %d in job %q has with but no uses", i+1, jobName)
				}
				if !validShells[step.Shell] {
					return fmt.Errorf("step %d in job %q has invalid shell %q: must be sh or bash", i+1, jobName, step.Shell)
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
