package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "flow",
	Short: "A task orchestration CLI tool",
	Long:  `flow is a task orchestration CLI tool that runs workflows defined in YAML configuration files, similar to GitHub Actions.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
