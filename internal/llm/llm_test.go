package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClaudeGenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("x-api-key = %s, want test-key", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("anthropic-version = %s, want 2023-06-01", r.Header.Get("anthropic-version"))
		}

		var req claudeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.System != "system prompt" {
			t.Fatalf("system = %q, want 'system prompt'", req.System)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content != "user query" {
			t.Fatalf("messages = %v, want [{user 'user query'}]", req.Messages)
		}

		_ = json.NewEncoder(w).Encode(claudeResponse{
			Content: []struct {
				Text string `json:"text"`
			}{{Text: "LLM answer"}},
		})
	}))
	defer server.Close()

	c := &Claude{
		APIKey:     "test-key",
		Model:      "test-model",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	got, err := c.Generate(context.Background(), "system prompt", "user query")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "LLM answer" {
		t.Fatalf("result = %q, want 'LLM answer'", got)
	}
}

func TestClaudeGenerateNoAPIKey(t *testing.T) {
	c := &Claude{APIKey: "", Model: "test"}
	_, err := c.Generate(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestClaudeGenerateAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limited"},
		})
	}))
	defer server.Close()

	c := &Claude{
		APIKey:     "key",
		Model:      "test",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := c.Generate(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClaudeGenerateEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(claudeResponse{Content: nil})
	}))
	defer server.Close()

	c := &Claude{
		APIKey:     "key",
		Model:      "test",
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}

	_, err := c.Generate(context.Background(), "", "hello")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestNewClaudeDefaults(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	c := NewClaude("", "")
	if c.APIKey != "env-key" {
		t.Fatalf("APIKey = %q, want env-key", c.APIKey)
	}
	if c.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("Model = %q, want claude-sonnet-4-20250514", c.Model)
	}
}

func TestNewClaudeCLIDefaults(t *testing.T) {
	c := NewClaudeCLI("opus")
	if c.Model != "opus" {
		t.Fatalf("Model = %q, want opus", c.Model)
	}
}
