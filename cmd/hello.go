package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var helloCmd = &cobra.Command{
	Use:   "hello [name]",
	Short: "Print a greeting message",
	Long:  `Print a greeting message. If a name is provided, it greets the specified name. Otherwise, it greets "World".`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := "World"
		if len(args) > 0 {
			name = args[0]
		}
		fmt.Printf("Hello, %s!\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(helloCmd)
}
