package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/longkey1/flow/internal/workflow"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe <workflow>",
	Short: "Show workflow details",
	Long:  `Show details of a workflow including its inputs, jobs, and steps.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workflowName := args[0]

		baseDir, err := os.Getwd()
		if err != nil {
			return err
		}

		path, err := workflow.Find(workflowsDir(baseDir), workflowName)
		if err != nil {
			return err
		}

		wf, err := workflow.Load(path)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Workflow: %s\n", wf.Name)

		if len(wf.Inputs) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Inputs:")

			names := make([]string, 0, len(wf.Inputs))
			for name := range wf.Inputs {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				input := wf.Inputs[name]
				marker := ""
				if input.Required {
					marker = " (required)"
				}
				fmt.Fprintf(out, "  %s%s\n", name, marker)
				if input.Description != "" {
					fmt.Fprintf(out, "      %s\n", input.Description)
				}
				if input.Default != "" {
					fmt.Fprintf(out, "      default: %s\n", input.Default)
				}
			}
		}

		if len(wf.Outputs) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Outputs:")

			outputNames := make([]string, 0, len(wf.Outputs))
			for name := range wf.Outputs {
				outputNames = append(outputNames, name)
			}
			sort.Strings(outputNames)

			for _, name := range outputNames {
				fmt.Fprintf(out, "  %s: %s\n", name, wf.Outputs[name])
			}
		}

		if len(wf.Jobs) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Jobs:")
			for _, jobName := range wf.JobOrder {
				job := wf.Jobs[jobName]
				parts := []string{}
				if len(job.Needs) > 0 {
					parts = append(parts, "needs: "+join(job.Needs))
				}
				if job.Uses != "" {
					parts = append(parts, "uses: "+job.Uses)
				}
				fmt.Fprintf(out, "  %s", jobName)
				if len(parts) > 0 {
					fmt.Fprintf(out, " (%s)", strings.Join(parts, ", "))
				}
				fmt.Fprintln(out)
				if job.Strategy != nil {
					fmt.Fprintln(out, "    strategy:")
					fmt.Fprintln(out, "      matrix:")
					for k, param := range job.Strategy.Matrix {
						if param.Expression != "" {
							fmt.Fprintf(out, "        %s: %s\n", k, param.Expression)
						} else {
							fmt.Fprintf(out, "        %s: [%s]\n", k, join(param.Values))
						}
					}
				}
				if job.Uses != "" && len(job.With) > 0 {
					fmt.Fprintln(out, "    with:")
					for k, v := range job.With {
						fmt.Fprintf(out, "      %s: %s\n", k, v)
					}
				}
				for _, step := range job.Steps {
					name := step.Name
					if name == "" {
						if step.Uses != "" {
							name = "uses: " + step.Uses
						} else {
							name = step.Run
						}
					}
					fmt.Fprintf(out, "    - %s\n", name)
					if step.Uses != "" && len(step.With) > 0 {
						for k, v := range step.With {
							fmt.Fprintf(out, "        %s: %s\n", k, v)
						}
					}
				}
			}
		}

		return nil
	},
}

func join(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

func init() {
	rootCmd.AddCommand(describeCmd)
}
