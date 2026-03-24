package cli

import (
	"log/slog"
	"os"

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
		initLogger()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.AddCommand(ingestCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(updateCheckCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(configCmd)
}

func Execute() error {
	return rootCmd.Execute()
}

func initLogger() {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
