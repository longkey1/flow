package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/longkey1/flow/internal/runner"
	"github.com/longkey1/flow/internal/workflow"
	"github.com/spf13/cobra"
)

func workflowsDir(baseDir string) string {
	root := os.Getenv("FLOW_ROOT")
	if root == "" {
		root = ".flow"
	}
	return filepath.Join(baseDir, root, "workflows")
}

func actionsDir(baseDir string) string {
	root := os.Getenv("FLOW_ROOT")
	if root == "" {
		root = ".flow"
	}
	return filepath.Join(baseDir, root, "actions")
}

func parseInputFlags(raw []string) (map[string]string, error) {
	inputs := make(map[string]string)
	for _, s := range raw {
		idx := strings.IndexByte(s, '=')
		if idx < 1 {
			return nil, fmt.Errorf("invalid input format %q: must be key=value", s)
		}
		inputs[s[:idx]] = s[idx+1:]
	}
	return inputs, nil
}

var runCmd = &cobra.Command{
	Use:   "run <workflow>",
	Short: "Run a workflow",
	Long:  `Run a workflow defined in the workflows directory.`,
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

		inputFlags, _ := cmd.Flags().GetStringArray("input")
		inputs, err := parseInputFlags(inputFlags)
		if err != nil {
			return err
		}

		debug, _ := cmd.Flags().GetBool("debug")
		r := runner.New(os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr(), baseDir)
		r.Quiet = wf.Quiet && !debug
		r.ActionsDir = actionsDir(baseDir)
		return r.Run(wf, inputs)
	},
}

func init() {
	runCmd.Flags().Bool("debug", false, "Show detailed output regardless of workflow quiet setting")
	runCmd.Flags().StringArrayP("input", "i", nil, "Input values in key=value format (can be specified multiple times)")
	rootCmd.AddCommand(runCmd)
}
