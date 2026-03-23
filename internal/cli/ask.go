package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var askCmd = &cobra.Command{
	Use:   "ask <query>",
	Short: "Ask a question and get a summary-based answer with citations",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("ask: not yet implemented (query: %s)\n", args[0])
		return nil
	},
}
