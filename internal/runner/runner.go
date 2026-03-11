package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/longkey1/flow/internal/action"
	"github.com/longkey1/flow/internal/workflow"
)

const maxWorkflowDepth = 10

type Runner struct {
	stdin        io.Reader
	stdout       io.Writer
	stderr       io.Writer
	dir          string
	Quiet        bool
	ActionsDir   string
	WorkflowsDir string
}

func New(stdin io.Reader, stdout, stderr io.Writer, dir string) *Runner {
	return &Runner{stdin: stdin, stdout: stdout, stderr: stderr, dir: dir}
}

func (r *Runner) Run(wf *workflow.Workflow, inputs map[string]string) error {
	_, err := r.run(wf, inputs, 0)
	return err
}

func (r *Runner) run(wf *workflow.Workflow, inputs map[string]string, depth int) (map[string]string, error) {
	if depth > maxWorkflowDepth {
		return nil, fmt.Errorf("maximum workflow depth %d exceeded", maxWorkflowDepth)
	}

	// Resolve inputs: apply defaults and validate required
	resolvedInputs, err := resolveInputs(wf, inputs)
	if err != nil {
		return nil, err
	}

	status := make(map[string]string) // "success", "failed", "skipped"
	jobOutputs := make(map[string]map[string]string)
	var failedJobs []string
	var mu sync.Mutex

	// Create done channels for each job
	done := make(map[string]chan struct{})
	for _, jobName := range wf.JobOrder {
		done[jobName] = make(chan struct{})
	}

	var wg sync.WaitGroup
	for _, jobName := range wf.JobOrder {
		wg.Add(1)
		go func(jobName string) {
			defer wg.Done()
			defer close(done[jobName])

			job := wf.Jobs[jobName]

			// Wait for dependencies
			for _, need := range job.Needs {
				<-done[need]
			}

			// Check if all dependencies succeeded
			mu.Lock()
			skip := false
			for _, need := range job.Needs {
				if status[need] != "success" {
					skip = true
					break
				}
			}
			if skip {
				status[jobName] = "skipped"
				mu.Unlock()
				if !r.Quiet {
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "=== Job: %s (skipped) ===\n", jobName)
					mu.Lock()
					buf.WriteTo(r.stdout)
					mu.Unlock()
				}
				return
			}
			mu.Unlock()

			// Handle job-level uses (reusable workflows)
			if job.Uses != "" {
				if !r.Quiet {
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "=== Job: %s (uses: %s) ===\n", jobName, job.Uses)
					mu.Lock()
					buf.WriteTo(r.stdout)
					mu.Unlock()
				}

				mu.Lock()
				currentJobOutputs := copyJobOutputs(jobOutputs)
				mu.Unlock()

				subOutputs, err := r.runSubWorkflow(job, wf.Env, resolvedInputs, currentJobOutputs, depth)
				mu.Lock()
				if err != nil {
					fmt.Fprintf(r.stderr, "job %q: %v\n", jobName, err)
					status[jobName] = "failed"
					failedJobs = append(failedJobs, jobName)
				} else {
					if subOutputs != nil {
						jobOutputs[jobName] = subOutputs
					}
					status[jobName] = "success"
				}
				mu.Unlock()
				return
			}

			// Per-job buffers
			var stdoutBuf, stderrBuf bytes.Buffer

			if !r.Quiet {
				fmt.Fprintf(&stdoutBuf, "=== Job: %s ===\n", jobName)
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
					fmt.Fprintf(&stdoutBuf, "--- Step: %s ---\n", name)
				}

				if step.Uses != "" {
					mu.Lock()
					actionJobOutputs := copyJobOutputs(jobOutputs)
					mu.Unlock()
					outputs, err := r.runActionBuffered(step, jobEnv, stepOutputs, resolvedInputs, actionJobOutputs, &stdoutBuf, &stderrBuf)
					if err != nil {
						jobFailed = true
						fmt.Fprintf(&stderrBuf, "job %q, step %q: %v\n", jobName, name, err)
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
					jobFailed = true
					fmt.Fprintf(&stderrBuf, "job %q, step %q: creating output file: %v\n", jobName, name, err)
					break
				}
				outputPath := outputFile.Name()
				outputFile.Close()

				// Expand expressions in the command
				mu.Lock()
				currentJobOutputs := copyJobOutputs(jobOutputs)
				mu.Unlock()
				command := expandExpressions(step.Run, stepOutputs, resolvedInputs, currentJobOutputs)

				// Merge workflow env → job env → step env
				stepEnv := mergeEnv(jobEnv, step.Env)
				env := make([]string, 0, len(stepEnv)+1)
				for k, v := range stepEnv {
					env = append(env, k+"="+v)
				}
				env = append(env, "FLOW_OUTPUT="+outputPath)
				if err := runShell(command, r.dir, r.stdin, &stdoutBuf, &stderrBuf, env); err != nil {
					os.Remove(outputPath)
					jobFailed = true
					fmt.Fprintf(&stderrBuf, "job %q, step %q: %v\n", jobName, name, err)
					break
				}

				// Parse outputs if step has an id
				if step.Id != "" {
					outputs, err := parseOutputFile(outputPath)
					if err != nil {
						os.Remove(outputPath)
						jobFailed = true
						fmt.Fprintf(&stderrBuf, "job %q, step %q: parsing output: %v\n", jobName, name, err)
						break
					}
					stepOutputs[step.Id] = outputs
				}
				os.Remove(outputPath)
			}

			// Resolve job outputs using step outputs
			if !jobFailed && len(job.Outputs) > 0 {
				resolved := make(map[string]string, len(job.Outputs))
				for k, v := range job.Outputs {
					resolved[k] = expandExpressions(v, stepOutputs, resolvedInputs, nil)
				}
				mu.Lock()
				jobOutputs[jobName] = resolved
				mu.Unlock()
			}

			// Flush buffers and update status atomically
			mu.Lock()
			stdoutBuf.WriteTo(r.stdout)
			stderrBuf.WriteTo(r.stderr)
			if jobFailed {
				status[jobName] = "failed"
				failedJobs = append(failedJobs, jobName)
			} else {
				status[jobName] = "success"
			}
			mu.Unlock()
		}(jobName)
	}

	wg.Wait()

	if len(failedJobs) > 0 {
		return nil, fmt.Errorf("jobs failed: %v", failedJobs)
	}

	// Resolve workflow-level outputs
	if len(wf.Outputs) > 0 {
		resolved := make(map[string]string, len(wf.Outputs))
		for k, v := range wf.Outputs {
			resolved[k] = expandWorkflowOutputs(v, jobOutputs)
		}
		return resolved, nil
	}

	return nil, nil
}

func (r *Runner) runSubWorkflow(job workflow.Job, callerEnv map[string]string, callerInputs map[string]string, callerJobOutputs map[string]map[string]string, depth int) (map[string]string, error) {
	usesName := strings.TrimPrefix(job.Uses, "./")

	subPath, err := workflow.Find(r.WorkflowsDir, usesName)
	if err != nil {
		return nil, fmt.Errorf("sub-workflow %q: %w", usesName, err)
	}
	subWf, err := workflow.Load(subPath)
	if err != nil {
		return nil, fmt.Errorf("sub-workflow %q: %w", usesName, err)
	}

	// Resolve with values using caller context
	resolvedWith := make(map[string]string, len(job.With))
	for k, v := range job.With {
		resolvedWith[k] = expandExpressions(v, nil, callerInputs, callerJobOutputs)
	}

	// Merge caller env into sub-workflow env (sub-workflow env takes precedence)
	subWf.Env = mergeEnv(callerEnv, subWf.Env)

	// Create sub-runner
	subRunner := &Runner{
		stdin:        r.stdin,
		stdout:       r.stdout,
		stderr:       r.stderr,
		dir:          r.dir,
		Quiet:        r.Quiet,
		ActionsDir:   r.ActionsDir,
		WorkflowsDir: r.WorkflowsDir,
	}

	return subRunner.run(subWf, resolvedWith, depth+1)
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

func (r *Runner) runAction(step workflow.Step, jobEnv map[string]string, stepOutputs map[string]map[string]string, workflowInputs map[string]string, jobOutputs map[string]map[string]string) (map[string]string, error) {
	return r.runActionBuffered(step, jobEnv, stepOutputs, workflowInputs, jobOutputs, r.stdout, r.stderr)
}

func (r *Runner) runActionBuffered(step workflow.Step, jobEnv map[string]string, stepOutputs map[string]map[string]string, workflowInputs map[string]string, jobOutputs map[string]map[string]string, stdout, stderr io.Writer) (map[string]string, error) {
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
		resolvedWith[k] = expandExpressions(v, stepOutputs, workflowInputs, jobOutputs)
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
		command := expandExpressions(actionStep.Run, actionStepOutputs, actionInputs, nil)

		// Merge env: calling step env → action step env
		stepEnv := mergeEnv(callingStepEnv, actionStep.Env)
		env := make([]string, 0, len(stepEnv)+1)
		for k, v := range stepEnv {
			env = append(env, k+"="+v)
		}
		env = append(env, "FLOW_OUTPUT="+outputPath)

		if err := runShell(command, r.dir, r.stdin, stdout, stderr, env); err != nil {
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

// copyJobOutputs creates a shallow copy of the jobOutputs map for safe concurrent access.
func copyJobOutputs(src map[string]map[string]string) map[string]map[string]string {
	dst := make(map[string]map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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
