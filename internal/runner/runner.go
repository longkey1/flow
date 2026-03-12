package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
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
	Debug        bool
	LogDir       string
	LogMaxRuns   int
	ActionsDir   string
	WorkflowsDir string
}

func New(stdin io.Reader, stdout, stderr io.Writer, dir string) *Runner {
	return &Runner{stdin: stdin, stdout: stdout, stderr: stderr, dir: dir}
}

func (r *Runner) Run(wf *workflow.Workflow, inputs map[string]string) error {
	var logFile *LogFile
	if r.LogDir != "" {
		var err error
		logFile, err = NewLogFile(r.LogDir, wf.Name)
		if err != nil {
			return err
		}
		defer logFile.Close()
	}

	_, err := r.run(wf, inputs, 0, logFile)

	if logFile != nil {
		if !r.Quiet {
			fmt.Fprintf(r.stdout, "\nLog: %s\n", logFile.Path())
		}
		if rotErr := RotateLogs(r.LogDir, r.LogMaxRuns); rotErr != nil {
			fmt.Fprintf(r.stderr, "warning: log rotation failed: %v\n", rotErr)
		}
	}

	return err
}

func (r *Runner) run(wf *workflow.Workflow, inputs map[string]string, depth int, logFile *LogFile) (map[string]string, error) {
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

			// Check dependencies and evaluate job-level if condition
			mu.Lock()
			anyDepFailed := false
			for _, need := range job.Needs {
				if status[need] != "success" {
					anyDepFailed = true
					break
				}
			}

			skip := false
			if job.If != "" {
				// Evaluate job-level if condition
				shouldRun, err := evaluateCondition(job.If, anyDepFailed, nil, resolvedInputs, copyJobOutputs(jobOutputs), nil)
				if err != nil {
					fmt.Fprintf(r.stderr, "job %q: %v\n", jobName, err)
					status[jobName] = "failed"
					failedJobs = append(failedJobs, jobName)
					mu.Unlock()
					return
				}
				if !shouldRun {
					skip = true
				}
			} else if anyDepFailed {
				// Default behavior: skip when any dependency failed
				skip = true
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

			// Matrix strategy
			if job.Strategy != nil {
				mu.Lock()
				currentJobOutputs := copyJobOutputs(jobOutputs)
				mu.Unlock()

				// Resolve all matrix params
				resolvedParams := make(map[string][]string)
				for key, param := range job.Strategy.Matrix {
					values, err := resolveMatrixParam(param, resolvedInputs, currentJobOutputs)
					if err != nil {
						mu.Lock()
						fmt.Fprintf(r.stderr, "job %q: resolving matrix param %q: %v\n", jobName, key, err)
						status[jobName] = "failed"
						failedJobs = append(failedJobs, jobName)
						mu.Unlock()
						return
					}
					resolvedParams[key] = values
				}

				combos := cartesianProduct(resolvedParams)

				var matrixWg sync.WaitGroup
				var matrixFailed int32
				var matrixMu sync.Mutex

				for _, combo := range combos {
					matrixWg.Add(1)
					go func(matrixValues map[string]string) {
						defer matrixWg.Done()

						matrixLabel := formatMatrixLabel(matrixValues)

						if job.Uses != "" {
							var stdoutBuf, stderrBuf bytes.Buffer
							if !r.Quiet {
								fmt.Fprintf(&stdoutBuf, "=== Job: %s [%s] (uses: %s) ===\n", jobName, matrixLabel, job.Uses)
							}

							bufRunner := &Runner{
								stdin:        r.stdin,
								stdout:       &stdoutBuf,
								stderr:       &stderrBuf,
								dir:          r.dir,
								Quiet:        r.Quiet,
								Debug:        r.Debug,
								LogDir:       r.LogDir,
								ActionsDir:   r.ActionsDir,
								WorkflowsDir: r.WorkflowsDir,
							}
							_, err := bufRunner.runSubWorkflow(job, wf.Env, resolvedInputs, currentJobOutputs, matrixValues, depth, logFile)
							if err != nil {
								fmt.Fprintf(&stderrBuf, "job %q [%s]: %v\n", jobName, matrixLabel, err)
								matrixMu.Lock()
								matrixFailed++
								matrixMu.Unlock()
							}
							mu.Lock()
							stdoutBuf.WriteTo(r.stdout)
							stderrBuf.WriteTo(r.stderr)
							mu.Unlock()
						} else {
							var stdoutBuf, stderrBuf bytes.Buffer
							if !r.Quiet {
								fmt.Fprintf(&stdoutBuf, "=== Job: %s [%s] ===\n", jobName, matrixLabel)
							}

							_, failed := r.runJobSteps(job, jobName, wf.Env, resolvedInputs, currentJobOutputs, matrixValues, &stdoutBuf, &stderrBuf, logFile)

							mu.Lock()
							stdoutBuf.WriteTo(r.stdout)
							stderrBuf.WriteTo(r.stderr)
							mu.Unlock()

							if failed {
								matrixMu.Lock()
								matrixFailed++
								matrixMu.Unlock()
							}
						}
					}(combo)
				}

				matrixWg.Wait()

				mu.Lock()
				if matrixFailed > 0 {
					status[jobName] = "failed"
					failedJobs = append(failedJobs, jobName)
				} else {
					status[jobName] = "success"
				}
				mu.Unlock()
				return
			}

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

				subOutputs, err := r.runSubWorkflow(job, wf.Env, resolvedInputs, currentJobOutputs, nil, depth, logFile)
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

			mu.Lock()
			currentJobOutputs := copyJobOutputs(jobOutputs)
			mu.Unlock()

			stepOutputs, jobFailed := r.runJobSteps(job, jobName, wf.Env, resolvedInputs, currentJobOutputs, nil, &stdoutBuf, &stderrBuf, logFile)

			// Resolve job outputs using step outputs
			if !jobFailed && len(job.Outputs) > 0 {
				resolved := make(map[string]string, len(job.Outputs))
				for k, v := range job.Outputs {
					resolved[k] = expandExpressions(v, stepOutputs, resolvedInputs, nil, nil)
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

// runJobSteps executes all steps in a job and returns step outputs and whether the job failed.
func (r *Runner) runJobSteps(job workflow.Job, jobName string, wfEnv map[string]string,
	resolvedInputs map[string]string, jobOutputs map[string]map[string]string,
	matrixValues map[string]string, stdout, stderr io.Writer, logFile *LogFile) (map[string]map[string]string, bool) {

	// Merge workflow env → job env
	jobEnv := mergeEnv(wfEnv, job.Env)

	// Resolve default shell from job defaults
	var defaultShell string
	if job.Defaults != nil {
		defaultShell = job.Defaults.Run.Shell
	}

	// Wrap stdout/stderr with indentation for step-level output
	stepStdout := io.Writer(newIndentWriter(stdout, "  "))
	stepStderr := io.Writer(newIndentWriter(stderr, "  "))

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

		// Evaluate if condition
		if step.If != "" {
			shouldRun, err := evaluateCondition(step.If, jobFailed, stepOutputs, resolvedInputs, jobOutputs, matrixValues)
			if err != nil {
				jobFailed = true
				fmt.Fprintf(stepStderr, "job %q, step %q: %v\n", jobName, name, err)
				continue
			}
			if !shouldRun {
				if !r.Quiet {
					fmt.Fprintf(stepStdout, "--- Step: %s (skipped) ---\n", name)
				}
				continue
			}
		} else if jobFailed {
			// Default behavior: skip when previous steps failed (same as success())
			if !r.Quiet {
				fmt.Fprintf(stepStdout, "--- Step: %s (skipped) ---\n", name)
			}
			continue
		}

		if !r.Quiet {
			fmt.Fprintf(stepStdout, "--- Step: %s ---\n", name)
		}

		if step.Uses != "" {
			outputs, err := r.runActionBuffered(step, jobEnv, stepOutputs, resolvedInputs, jobOutputs, matrixValues, stepStdout, stepStderr, logFile, jobName, name)
			if err != nil {
				jobFailed = true
				fmt.Fprintf(stepStderr, "job %q, step %q: %v\n", jobName, name, err)
				continue
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
			fmt.Fprintf(stepStderr, "job %q, step %q: creating output file: %v\n", jobName, name, err)
			continue
		}
		outputPath := outputFile.Name()
		outputFile.Close()

		// Expand expressions in the command
		command := expandExpressions(step.Run, stepOutputs, resolvedInputs, jobOutputs, matrixValues)

		// Merge workflow env → job env → step env
		stepEnv := mergeEnv(jobEnv, step.Env)
		env := make([]string, 0, len(stepEnv)+1)
		for k, v := range stepEnv {
			env = append(env, k+"="+v)
		}
		env = append(env, "FLOW_OUTPUT="+outputPath)
		// Resolve shell: step.Shell > job defaults > "sh"
		shell := step.Shell
		if shell == "" {
			shell = defaultShell
		}

		// Determine stdout/stderr writers for the shell command
		shellStdout, shellStderr := r.wrapWriters(stepStdout, stepStderr, logFile, jobName+"/"+name)

		if err := runShell(command, r.dir, shell, r.stdin, shellStdout, shellStderr, env); err != nil {
			flushPrefixedWriter(shellStdout)
			flushPrefixedWriter(shellStderr)
			os.Remove(outputPath)
			jobFailed = true
			fmt.Fprintf(stepStderr, "job %q, step %q: %v\n", jobName, name, err)
			continue
		}

		flushPrefixedWriter(shellStdout)
		flushPrefixedWriter(shellStderr)

		// Parse outputs if step has an id
		if step.Id != "" {
			outputs, err := parseOutputFile(outputPath)
			if err != nil {
				os.Remove(outputPath)
				jobFailed = true
				fmt.Fprintf(stepStderr, "job %q, step %q: parsing output: %v\n", jobName, name, err)
				continue
			}
			stepOutputs[step.Id] = outputs
		}
		os.Remove(outputPath)
	}

	return stepOutputs, jobFailed
}

func (r *Runner) runSubWorkflow(job workflow.Job, callerEnv map[string]string, callerInputs map[string]string, callerJobOutputs map[string]map[string]string, matrixValues map[string]string, depth int, logFile *LogFile) (map[string]string, error) {
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
		resolvedWith[k] = expandExpressions(v, nil, callerInputs, callerJobOutputs, matrixValues)
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
		Debug:        r.Debug,
		LogDir:       r.LogDir,
		ActionsDir:   r.ActionsDir,
		WorkflowsDir: r.WorkflowsDir,
	}

	return subRunner.run(subWf, resolvedWith, depth+1, logFile)
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

func (r *Runner) runActionBuffered(step workflow.Step, jobEnv map[string]string, stepOutputs map[string]map[string]string, workflowInputs map[string]string, jobOutputs map[string]map[string]string, matrixValues map[string]string, stdout, stderr io.Writer, logFile *LogFile, jobName, callerStepName string) (map[string]string, error) {
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
		resolvedWith[k] = expandExpressions(v, stepOutputs, workflowInputs, jobOutputs, matrixValues)
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
		// Resolve sub-step name for display
		subStepName := actionStep.Name
		if subStepName == "" {
			subStepName = actionStep.Run
		}

		// Show sub-step name on screen when logging is enabled
		if logFile != nil && !r.Quiet {
			fmt.Fprintf(stdout, "  > %s\n", subStepName)
		}

		// Create temp file for FLOW_OUTPUT
		outputFile, err := os.CreateTemp("", "flow-output-*")
		if err != nil {
			return nil, fmt.Errorf("creating output file: %w", err)
		}
		outputPath := outputFile.Name()
		outputFile.Close()

		// Expand expressions: inputs.X → actionInputs, steps.X.outputs.Y → action step outputs
		command := expandExpressions(actionStep.Run, actionStepOutputs, actionInputs, nil, nil)

		// Merge env: calling step env → action step env
		stepEnv := mergeEnv(callingStepEnv, actionStep.Env)
		env := make([]string, 0, len(stepEnv)+1)
		for k, v := range stepEnv {
			env = append(env, k+"="+v)
		}
		env = append(env, "FLOW_OUTPUT="+outputPath)

		// Determine stdout/stderr writers for the shell command (indented within sub-step)
		subStepStdout := io.Writer(newIndentWriter(stdout, "  "))
		subStepStderr := io.Writer(newIndentWriter(stderr, "  "))
		prefix := jobName + "/" + callerStepName + " > " + subStepName
		shellStdout, shellStderr := r.wrapWriters(subStepStdout, subStepStderr, logFile, prefix)

		if err := runShell(command, r.dir, "", r.stdin, shellStdout, shellStderr, env); err != nil {
			flushPrefixedWriter(shellStdout)
			flushPrefixedWriter(shellStderr)
			os.Remove(outputPath)
			return nil, err
		}

		flushPrefixedWriter(shellStdout)
		flushPrefixedWriter(shellStderr)

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

// wrapWriters returns stdout/stderr writers that route output through PrefixedWriter when logging is enabled.
// When logFile is nil, returns the original writers unchanged.
func (r *Runner) wrapWriters(stdout, stderr io.Writer, logFile *LogFile, prefix string) (io.Writer, io.Writer) {
	if logFile == nil {
		return stdout, stderr
	}

	logStdout := NewPrefixedWriter(logFile, prefix)
	logStderr := NewPrefixedWriter(logFile, prefix)

	if r.Debug {
		return io.MultiWriter(stdout, logStdout), io.MultiWriter(stderr, logStderr)
	}
	return logStdout, logStderr
}

// flushPrefixedWriter flushes a writer if it is a *PrefixedWriter or wraps one.
func flushPrefixedWriter(w io.Writer) {
	if pw, ok := w.(*PrefixedWriter); ok {
		pw.Flush()
		return
	}
	// Handle io.MultiWriter: check if underlying writers include a PrefixedWriter.
	// Since io.MultiWriter is opaque, we use a type assertion on a known wrapper interface.
	// Instead, we accept a flusher interface.
	type flusher interface {
		Flush() error
	}
	if f, ok := w.(flusher); ok {
		f.Flush()
	}
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

// formatMatrixLabel formats matrix values as "key=val, key2=val2" for display.
func formatMatrixLabel(matrixValues map[string]string) string {
	keys := make([]string, 0, len(matrixValues))
	for k := range matrixValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+matrixValues[k])
	}
	return strings.Join(parts, ", ")
}
