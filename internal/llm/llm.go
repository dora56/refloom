package llm

import "context"

// Provider generates text responses from a language model.
type Provider interface {
	Generate(ctx context.Context, system, user string) (string, error)
}
