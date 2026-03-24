package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	// Redact sensitive fields
	display := *cfg
	if display.AnthropicAPIKey != "" {
		display.AnthropicAPIKey = "***"
	}

	data, err := yaml.Marshal(&display)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	fmt.Print(string(data))
	return nil
}
