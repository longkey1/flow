package runner

import (
	"fmt"
	"io"
	"os"

	"github.com/longkey1/flow/internal/workflow"
)

type Runner struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	dir    string
}

func New(stdin io.Reader, stdout, stderr io.Writer, dir string) *Runner {
	return &Runner{stdin: stdin, stdout: stdout, stderr: stderr, dir: dir}
}

func (r *Runner) Run(wf *workflow.Workflow) error {
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
			fmt.Fprintf(r.stdout, "=== Job: %s (skipped) ===\n", jobName)
			continue
		}

		fmt.Fprintf(r.stdout, "=== Job: %s ===\n", jobName)

		stepOutputs := make(map[string]map[string]string)
		jobFailed := false
		for _, step := range job.Steps {
			name := step.Name
			if name == "" {
				name = step.Run
			}
			fmt.Fprintf(r.stdout, "--- Step: %s ---\n", name)

			// Create temp file for FLOW_OUTPUT
			outputFile, err := os.CreateTemp("", "flow-output-*")
			if err != nil {
				return fmt.Errorf("creating output file: %w", err)
			}
			outputPath := outputFile.Name()
			outputFile.Close()

			// Expand expressions in the command
			command := expandExpressions(step.Run, stepOutputs)

			env := []string{"FLOW_OUTPUT=" + outputPath}
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
