package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "refloom",
	Short: "Refloom - Local reading support RAG tool",
	Long:  "Refloom is a personal, local reading support RAG tool for PDF/EPUB books.",
}

func init() {
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(reindexCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
