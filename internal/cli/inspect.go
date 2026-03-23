package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [book-id]",
	Short: "Inspect book metadata, chapters, and chunks",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			fmt.Printf("inspect: not yet implemented (book-id: %s)\n", args[0])
		} else {
			fmt.Println("inspect: not yet implemented (listing all books)")
		}
		return nil
	},
}
