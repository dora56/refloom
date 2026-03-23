package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild search indexes (FTS and/or embeddings)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("reindex: not yet implemented")
		return nil
	},
}
