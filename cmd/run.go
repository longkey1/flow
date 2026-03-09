package cmd

import (
	"os"

	"github.com/longkey1/flow/internal/config"
	"github.com/longkey1/flow/internal/runner"
	"github.com/longkey1/flow/internal/workflow"
	"github.com/spf13/cobra"
)

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

		cfg, err := config.Load(baseDir)
		if err != nil {
			return err
		}

		path, err := workflow.Find(cfg.WorkflowsDir(baseDir), workflowName)
		if err != nil {
			return err
		}

		wf, err := workflow.Load(path)
		if err != nil {
			return err
		}

		r := runner.New(os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr(), baseDir)
		return r.Run(wf)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
