package llm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCLI implements the Provider interface using the Claude Code CLI.
type ClaudeCLI struct {
	Model string // optional model override (e.g., "sonnet", "opus")
}

// NewClaudeCLI creates a Claude CLI provider.
func NewClaudeCLI(model string) *ClaudeCLI {
	return &ClaudeCLI{Model: model}
}

func (c *ClaudeCLI) Generate(ctx context.Context, system, user string) (string, error) {
	// Build the prompt combining system and user messages
	prompt := user
	if system != "" {
		prompt = system + "\n\n" + user
	}

	args := []string{"--print"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude CLI failed: %w\nstderr: %s", err, stderr.String())
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", fmt.Errorf("claude CLI returned empty response")
	}

	return result, nil
}
