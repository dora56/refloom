package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <path>",
	Short: "Ingest PDF/EPUB books into the local database",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("ingest: not yet implemented (path: %s)\n", args[0])
		return nil
	},
}
