package cmd

import (
	"os"
	"path/filepath"

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

		debug, _ := cmd.Flags().GetBool("debug")
		r := runner.New(os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr(), baseDir)
		r.Quiet = wf.Quiet && !debug
		return r.Run(wf)
	},
}

func init() {
	runCmd.Flags().Bool("debug", false, "Show detailed output regardless of workflow quiet setting")
	rootCmd.AddCommand(runCmd)
}
