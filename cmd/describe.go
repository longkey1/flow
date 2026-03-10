package cmd

import (
	"fmt"
	"os"
	"sort"

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

		if len(wf.Jobs) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Jobs:")
			for _, jobName := range wf.JobOrder {
				job := wf.Jobs[jobName]
				fmt.Fprintf(out, "  %s", jobName)
				if len(job.Needs) > 0 {
					fmt.Fprintf(out, " (needs: %s)", join(job.Needs))
				}
				fmt.Fprintln(out)
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
