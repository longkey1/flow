package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/longkey1/flow/internal/workflow"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available workflows",
	Long:    `List all workflows found in the workflows directory.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		baseDir, err := os.Getwd()
		if err != nil {
			return err
		}

		wfDir := workflowsDir(baseDir)
		entries, err := os.ReadDir(wfDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("workflows directory not found: %s", wfDir)
			}
			return err
		}

		out := cmd.OutOrStdout()
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ext)
			path := filepath.Join(wfDir, entry.Name())

			wf, err := workflow.Load(path)
			if err != nil {
				fmt.Fprintf(out, "  %s (invalid: %v)\n", name, err)
				continue
			}
			fmt.Fprintf(out, "  %s - %s\n", name, wf.Name)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
