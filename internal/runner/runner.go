package runner

import (
	"bytes"
	"encoding/json"
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
	Format       string
	LogDir       string
	LogMaxRuns   int
	ActionsDir   string
	WorkflowsDir string
}

func New(stdin io.Reader, stdout, stderr io.Writer, dir string) *Runner {
	return &Runner{stdin: stdin, stdout: stdout, stderr: stderr, dir: dir}
}

func (r *Runner) Run(wf *workflow.Workflow, inputs map[string]string) error {
	jsonOutput := r.Format == "json"
	if jsonOutput {
		r.Quiet = true
	}

	var logFile *LogFile
	if r.LogDir != "" {
		var err error
		logFile, err = NewLogFile(r.LogDir, wf.Name)
		if err != nil {
			return err
		}
		defer logFile.Close()
	}

	rr, err := r.run(wf, inputs, 0, logFile)

	if logFile != nil {
		if !r.Quiet {
			fmt.Fprintf(r.stdout, "\nLog: %s\n", logFile.Path())
		}
		if rotErr := RotateLogs(r.LogDir, r.LogMaxRuns); rotErr != nil {
			fmt.Fprintf(r.stderr, "warning: log rotation failed: %v\n", rotErr)
		}
	}

	if jsonOutput {
		result := &Result{
			Workflow: wf.Name,
			Jobs:     make(map[string]*JobResult),
		}
		if rr != nil {
			for jobName, s := range rr.status {
				result.Jobs[jobName] = &JobResult{
					Status:  s,
					Outputs: rr.jobOutputs[jobName],
				}
			}
			result.Outputs = rr.outputs
		}
		if err != nil {
			result.Status = "failed"
		} else {
			result.Status = "success"
		}
		enc := json.NewEncoder(r.stdout)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(result); encErr != nil {
			return encErr
		}
	}

	return err
}

type runResult struct {
	outputs    map[string]string
	status     map[string]string
	jobOutputs map[string]map[string]string
}

func (r *Runner) run(wf *workflow.Workflow, inputs map[string]string, depth int, logFile *LogFile) (*runResult, error) {
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
					fmt.Fprintf(&buf, "[%s] Job: %s (skipped)\n", jobName, jobName)
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

				type matrixComboResult struct {
					matrixValues map[string]string
					stepOutputs  map[string]map[string]string // for direct steps
					subOutputs   map[string]string            // for uses (workflow-level outputs)
				}

				var matrixWg sync.WaitGroup
				var matrixFailed int32
				var matrixMu sync.Mutex
				var comboResults []matrixComboResult

				var sem chan struct{}
				if job.Strategy.MaxParallel > 0 {
					sem = make(chan struct{}, job.Strategy.MaxParallel)
				}

				for _, combo := range combos {
					matrixWg.Add(1)
					if sem != nil {
						sem <- struct{}{}
					}
					go func(matrixValues map[string]string) {
						defer matrixWg.Done()
						if sem != nil {
							defer func() { <-sem }()
						}

						matrixLabel := formatMatrixLabel(matrixValues)

						if job.Uses != "" {
							var stdoutBuf, stderrBuf bytes.Buffer
							if !r.Quiet {
								fmt.Fprintf(&stdoutBuf, "[%s %s] Job: %s [%s] (uses: %s)\n", jobName, matrixLabel, jobName, matrixLabel, job.Uses)
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
							res, err := bufRunner.runSubWorkflow(job, wf.Env, resolvedInputs, currentJobOutputs, matrixValues, depth, logFile)
							if err != nil {
								fmt.Fprintf(&stderrBuf, "job %q [%s]: %v\n", jobName, matrixLabel, err)
								matrixMu.Lock()
								matrixFailed++
								matrixMu.Unlock()
							} else {
								matrixMu.Lock()
								comboResults = append(comboResults, matrixComboResult{
									matrixValues: matrixValues,
									subOutputs:   res.outputs,
								})
								matrixMu.Unlock()
							}
							mu.Lock()
							stdoutBuf.WriteTo(r.stdout)
							stderrBuf.WriteTo(r.stderr)
							mu.Unlock()
						} else {
							var stdoutBuf, stderrBuf bytes.Buffer
							if !r.Quiet {
								fmt.Fprintf(&stdoutBuf, "[%s %s] Job: %s [%s]\n", jobName, matrixLabel, jobName, matrixLabel)
							}

							stepOutputs, failed := r.runJobSteps(job, jobName, jobName+" "+matrixLabel, wf.Env, resolvedInputs, currentJobOutputs, matrixValues, &stdoutBuf, &stderrBuf, logFile, wf.Defaults)

							if !failed {
								matrixMu.Lock()
								comboResults = append(comboResults, matrixComboResult{
									matrixValues: matrixValues,
									stepOutputs:  stepOutputs,
								})
								matrixMu.Unlock()
							}

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

					// Aggregate matrix combo outputs as JSON arrays
					if len(job.Outputs) > 0 {
						// Sort combo results by matrix label for deterministic output
						sort.Slice(comboResults, func(i, j int) bool {
							return formatMatrixLabel(comboResults[i].matrixValues) < formatMatrixLabel(comboResults[j].matrixValues)
						})

						resolved := make(map[string]string, len(job.Outputs))
						for outKey, outExpr := range job.Outputs {
							type matrixOutputEntry struct {
								Matrix map[string]string `json:"matrix"`
								Value  string            `json:"value"`
							}
							entries := make([]matrixOutputEntry, 0, len(comboResults))
							for _, cr := range comboResults {
								var val string
								if cr.subOutputs != nil {
									// uses: resolve from sub-workflow outputs
									val = cr.subOutputs[outKey]
								} else {
									// direct steps: resolve output expression with step outputs
									val = expandExpressions(outExpr, cr.stepOutputs, resolvedInputs, nil, cr.matrixValues)
								}
								entries = append(entries, matrixOutputEntry{
									Matrix: cr.matrixValues,
									Value:  val,
								})
							}
							jsonBytes, _ := json.Marshal(entries)
							resolved[outKey] = string(jsonBytes)
						}
						jobOutputs[jobName] = resolved
					}
				}
				mu.Unlock()
				return
			}

			// Handle job-level uses (reusable workflows)
			if job.Uses != "" {
				if !r.Quiet {
					var buf bytes.Buffer
					fmt.Fprintf(&buf, "[%s] Job: %s (uses: %s)\n", jobName, jobName, job.Uses)
					mu.Lock()
					buf.WriteTo(r.stdout)
					mu.Unlock()
				}

				mu.Lock()
				currentJobOutputs := copyJobOutputs(jobOutputs)
				mu.Unlock()

				subResult, err := r.runSubWorkflow(job, wf.Env, resolvedInputs, currentJobOutputs, nil, depth, logFile)
				mu.Lock()
				if err != nil {
					fmt.Fprintf(r.stderr, "job %q: %v\n", jobName, err)
					status[jobName] = "failed"
					failedJobs = append(failedJobs, jobName)
				} else {
					if subResult != nil && subResult.outputs != nil {
						jobOutputs[jobName] = subResult.outputs
					}
					status[jobName] = "success"
				}
				mu.Unlock()
				return
			}

			// Per-job buffers
			var stdoutBuf, stderrBuf bytes.Buffer

			if !r.Quiet {
				fmt.Fprintf(&stdoutBuf, "[%s] Job: %s\n", jobName, jobName)
			}

			mu.Lock()
			currentJobOutputs := copyJobOutputs(jobOutputs)
			mu.Unlock()

			stepOutputs, jobFailed := r.runJobSteps(job, jobName, jobName, wf.Env, resolvedInputs, currentJobOutputs, nil, &stdoutBuf, &stderrBuf, logFile, wf.Defaults)

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

	res := &runResult{
		status:     status,
		jobOutputs: jobOutputs,
	}

	if len(failedJobs) > 0 {
		return res, fmt.Errorf("jobs failed: %v", failedJobs)
	}

	// Resolve workflow-level outputs
	if len(wf.Outputs) > 0 {
		resolved := make(map[string]string, len(wf.Outputs))
		for k, v := range wf.Outputs {
			resolved[k] = expandWorkflowOutputs(v, jobOutputs)
		}
		res.outputs = resolved
	}

	return res, nil
}

// runJobSteps executes all steps in a job and returns step outputs and whether the job failed.
// jobPrefix is the display prefix for the job (e.g. "deploy" or "deploy os=linux").
func (r *Runner) runJobSteps(job workflow.Job, jobName string, jobPrefix string, wfEnv map[string]string,
	resolvedInputs map[string]string, jobOutputs map[string]map[string]string,
	matrixValues map[string]string, stdout, stderr io.Writer, logFile *LogFile, wfDefaults *workflow.Defaults) (map[string]map[string]string, bool) {

	// Merge workflow env → job env
	jobEnv := mergeEnv(wfEnv, job.Env)

	// Resolve default shell: job defaults > workflow defaults
	var defaultShell string
	if job.Defaults != nil {
		defaultShell = job.Defaults.Run.Shell
	}
	if defaultShell == "" && wfDefaults != nil {
		defaultShell = wfDefaults.Run.Shell
	}

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

		// Create per-step prefix writers: [jobPrefix > stepName]
		stepPrefix := fmt.Sprintf("[%s > %s] ", jobPrefix, name)
		stepStdout := io.Writer(newIndentWriter(stdout, stepPrefix))
		stepStderr := io.Writer(newIndentWriter(stderr, stepPrefix))

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
					fmt.Fprintf(stepStdout, "Step: %s (skipped)\n", name)
				}
				continue
			}
		} else if jobFailed {
			// Default behavior: skip when previous steps failed (same as success())
			if !r.Quiet {
				fmt.Fprintf(stepStdout, "Step: %s (skipped)\n", name)
			}
			continue
		}

		if !r.Quiet {
			fmt.Fprintf(stepStdout, "Step: %s\n", name)
		}

		if step.Uses != "" {
			outputs, err := r.runActionBuffered(step, jobEnv, stepOutputs, resolvedInputs, jobOutputs, matrixValues, stdout, stderr, logFile, jobPrefix, name)
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

func (r *Runner) runSubWorkflow(job workflow.Job, callerEnv map[string]string, callerInputs map[string]string, callerJobOutputs map[string]map[string]string, matrixValues map[string]string, depth int, logFile *LogFile) (*runResult, error) {
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

func (r *Runner) runActionBuffered(step workflow.Step, jobEnv map[string]string, stepOutputs map[string]map[string]string, workflowInputs map[string]string, jobOutputs map[string]map[string]string, matrixValues map[string]string, stdout, stderr io.Writer, logFile *LogFile, jobPrefix, callerStepName string) (map[string]string, error) {
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

	// Resolve default shell for action steps
	defaultShell := ""
	if act.Defaults != nil {
		defaultShell = act.Defaults.Run.Shell
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

		// Create per-substep prefix writers: [jobPrefix > stepName > subStepName]
		subStepPrefix := fmt.Sprintf("[%s > %s > %s] ", jobPrefix, callerStepName, subStepName)
		subStepStdout := io.Writer(newIndentWriter(stdout, subStepPrefix))
		subStepStderr := io.Writer(newIndentWriter(stderr, subStepPrefix))

		// Show sub-step name on screen when logging is enabled
		if logFile != nil && !r.Quiet {
			fmt.Fprintf(subStepStdout, "> %s\n", subStepName)
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

		// Determine stdout/stderr writers for the shell command
		logPrefix := jobPrefix + "/" + callerStepName + " > " + subStepName
		shellStdout, shellStderr := r.wrapWriters(subStepStdout, subStepStderr, logFile, logPrefix)

		// Resolve shell: step.Shell > action defaults > "sh"
		shell := actionStep.Shell
		if shell == "" {
			shell = defaultShell
		}

		if err := runShell(command, r.dir, shell, r.stdin, shellStdout, shellStderr, env); err != nil {
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
