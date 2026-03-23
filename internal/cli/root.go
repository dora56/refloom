package cli

import (
	"github.com/dora56/refloom/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfg     *config.Config
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:   "refloom",
	Short: "Refloom - Local reading support RAG tool",
	Long:  "Refloom is a personal, local reading support RAG tool for PDF/EPUB books.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cfg = config.Load()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(reindexCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
