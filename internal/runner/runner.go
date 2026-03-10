package runner

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/longkey1/flow/internal/action"
	"github.com/longkey1/flow/internal/workflow"
)

type Runner struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	dir        string
	Quiet      bool
	ActionsDir string
}

func New(stdin io.Reader, stdout, stderr io.Writer, dir string) *Runner {
	return &Runner{stdin: stdin, stdout: stdout, stderr: stderr, dir: dir}
}

func (r *Runner) Run(wf *workflow.Workflow, inputs map[string]string) error {
	// Resolve inputs: apply defaults and validate required
	resolvedInputs, err := resolveInputs(wf, inputs)
	if err != nil {
		return err
	}

	status := make(map[string]string) // "success", "failed", "skipped"
	var failedJobs []string

	for _, jobName := range wf.JobOrder {
		job := wf.Jobs[jobName]

		// Check if all dependencies succeeded
		skip := false
		for _, need := range job.Needs {
			if status[need] != "success" {
				skip = true
				break
			}
		}

		if skip {
			status[jobName] = "skipped"
			if !r.Quiet {
				fmt.Fprintf(r.stdout, "=== Job: %s (skipped) ===\n", jobName)
			}
			continue
		}

		if !r.Quiet {
			fmt.Fprintf(r.stdout, "=== Job: %s ===\n", jobName)
		}

		// Merge workflow env → job env
		jobEnv := mergeEnv(wf.Env, job.Env)

		stepOutputs := make(map[string]map[string]string)
		jobFailed := false
		for _, step := range job.Steps {
			name := step.Name
			if name == "" {
				if step.Uses != "" {
					name = "uses: " + step.Uses
				} else {
					name = step.Run
				}
			}
			if !r.Quiet {
				fmt.Fprintf(r.stdout, "--- Step: %s ---\n", name)
			}

			if step.Uses != "" {
				outputs, err := r.runAction(step, jobEnv, stepOutputs, resolvedInputs)
				if err != nil {
					status[jobName] = "failed"
					failedJobs = append(failedJobs, jobName)
					jobFailed = true
					fmt.Fprintf(r.stderr, "job %q, step %q: %v\n", jobName, name, err)
					break
				}
				if step.Id != "" {
					stepOutputs[step.Id] = outputs
				}
				continue
			}

			// Create temp file for FLOW_OUTPUT
			outputFile, err := os.CreateTemp("", "flow-output-*")
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			outputPath := outputFile.Name()
			outputFile.Close()

			// Expand expressions in the command
			command := expandExpressions(step.Run, stepOutputs, resolvedInputs)

			// Merge workflow env → job env → step env
			stepEnv := mergeEnv(jobEnv, step.Env)
			env := make([]string, 0, len(stepEnv)+1)
			for k, v := range stepEnv {
				env = append(env, k+"="+v)
			}
			env = append(env, "FLOW_OUTPUT="+outputPath)
			if err := runShell(command, r.dir, r.stdin, r.stdout, r.stderr, env); err != nil {
				os.Remove(outputPath)
				status[jobName] = "failed"
				failedJobs = append(failedJobs, jobName)
				jobFailed = true
				fmt.Fprintf(r.stderr, "job %q, step %q: %v\n", jobName, name, err)
				break
			}

			// Parse outputs if step has an id
			if step.Id != "" {
				outputs, err := parseOutputFile(outputPath)
				if err != nil {
					os.Remove(outputPath)
					return fmt.Errorf("parsing output for step %q: %w", step.Id, err)
				}
				stepOutputs[step.Id] = outputs
			}
			os.Remove(outputPath)
		}
		if !jobFailed {
			status[jobName] = "success"
		}
	}

	if len(failedJobs) > 0 {
		return fmt.Errorf("jobs failed: %v", failedJobs)
	}
	return nil
}

// resolveInputs validates and resolves input values, applying defaults where needed.
func resolveInputs(wf *workflow.Workflow, provided map[string]string) (map[string]string, error) {
	resolved := make(map[string]string)
	for name, def := range wf.Inputs {
		if val, ok := provided[name]; ok {
			resolved[name] = val
		} else if def.Default != "" {
			resolved[name] = def.Default
		} else if def.Required {
			return nil, fmt.Errorf("required input %q is not provided", name)
		}
	}
	return resolved, nil
}

func (r *Runner) runAction(step workflow.Step, jobEnv map[string]string, stepOutputs map[string]map[string]string, workflowInputs map[string]string) (map[string]string, error) {
	// Strip leading "./" from uses name
	usesName := strings.TrimPrefix(step.Uses, "./")

	// Find and load the action
	actionPath, err := action.Find(r.ActionsDir, usesName)
	if err != nil {
		return nil, err
	}
	act, err := action.Load(actionPath)
	if err != nil {
		return nil, err
	}

	// Resolve with values (expand workflow expressions first)
	resolvedWith := make(map[string]string, len(step.With))
	for k, v := range step.With {
		resolvedWith[k] = expandExpressions(v, stepOutputs, workflowInputs)
	}

	// Resolve action inputs: with values + defaults + required check
	actionInputs, err := resolveActionInputs(act, resolvedWith)
	if err != nil {
		return nil, fmt.Errorf("action %q: %w", usesName, err)
	}

	// Execute action steps
	actionStepOutputs := make(map[string]map[string]string)
	callingStepEnv := mergeEnv(jobEnv, step.Env)

	for _, actionStep := range act.Runs.Steps {
		// Create temp file for FLOW_OUTPUT
		outputFile, err := os.CreateTemp("", "flow-output-*")
		if err != nil {
			return nil, fmt.Errorf("creating output file: %w", err)
		}
		outputPath := outputFile.Name()
		outputFile.Close()

		// Expand expressions: inputs.X → actionInputs, steps.X.outputs.Y → action step outputs
		command := expandExpressions(actionStep.Run, actionStepOutputs, actionInputs)

		// Merge env: calling step env → action step env
		stepEnv := mergeEnv(callingStepEnv, actionStep.Env)
		env := make([]string, 0, len(stepEnv)+1)
		for k, v := range stepEnv {
			env = append(env, k+"="+v)
		}
		env = append(env, "FLOW_OUTPUT="+outputPath)

		if err := runShell(command, r.dir, r.stdin, r.stdout, r.stderr, env); err != nil {
			os.Remove(outputPath)
			return nil, err
		}

		// Parse outputs if step has an id
		if actionStep.Id != "" {
			outputs, err := parseOutputFile(outputPath)
			if err != nil {
				os.Remove(outputPath)
				return nil, fmt.Errorf("parsing output for action step %q: %w", actionStep.Id, err)
			}
			actionStepOutputs[actionStep.Id] = outputs
		}
		os.Remove(outputPath)
	}

	// Merge all action step outputs into a single map for the calling step
	allOutputs := make(map[string]string)
	for _, outputs := range actionStepOutputs {
		for k, v := range outputs {
			allOutputs[k] = v
		}
	}
	return allOutputs, nil
}

func resolveActionInputs(act *action.Action, with map[string]string) (map[string]string, error) {
	resolved := make(map[string]string)
	for name, def := range act.Inputs {
		if val, ok := with[name]; ok {
			resolved[name] = val
		} else if def.Default != "" {
			resolved[name] = def.Default
		} else if def.Required {
			return nil, fmt.Errorf("required input %q is not provided", name)
		}
	}
	return resolved, nil
}

// mergeEnv merges two env maps. Values in override take precedence.
func mergeEnv(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}
