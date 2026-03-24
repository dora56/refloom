package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q, want http://localhost:11434", cfg.OllamaURL)
	}
	if cfg.OllamaEmbedModel != "nomic-embed-text" {
		t.Errorf("OllamaEmbedModel = %q, want nomic-embed-text", cfg.OllamaEmbedModel)
	}
	if cfg.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want 500", cfg.ChunkSize)
	}
	if cfg.Timeouts.Ingest != 30*time.Minute {
		t.Errorf("Timeouts.Ingest = %v, want 30m", cfg.Timeouts.Ingest)
	}
	if cfg.PromptBudget != 3000 {
		t.Errorf("PromptBudget = %d, want 3000", cfg.PromptBudget)
	}
}

func TestLoadFromYAML(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}

	yaml := `
db_path: /tmp/test.db
ollama_url: http://custom:1234
chunk_size: 1000
timeouts:
  search: 10s
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("DBPath = %q, want /tmp/test.db", cfg.DBPath)
	}
	if cfg.OllamaURL != "http://custom:1234" {
		t.Errorf("OllamaURL = %q, want http://custom:1234", cfg.OllamaURL)
	}
	if cfg.ChunkSize != 1000 {
		t.Errorf("ChunkSize = %d, want 1000", cfg.ChunkSize)
	}
	if cfg.Timeouts.Search != 10*time.Second {
		t.Errorf("Timeouts.Search = %v, want 10s", cfg.Timeouts.Search)
	}
	// Unset fields should keep defaults
	if cfg.OllamaEmbedModel != "nomic-embed-text" {
		t.Errorf("OllamaEmbedModel = %q, want nomic-embed-text (default)", cfg.OllamaEmbedModel)
	}
}

func TestLoadEnvVarOverride(t *testing.T) {
	// Use empty HOME to skip YAML loading
	t.Setenv("HOME", t.TempDir())

	t.Setenv("REFLOOM_DB_PATH", "/env/test.db")
	t.Setenv("REFLOOM_OLLAMA_URL", "http://env:5678")
	t.Setenv("REFLOOM_EMBEDDING_MODEL", "custom-model")
	t.Setenv("REFLOOM_LLM_PROVIDER", "ollama")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("REFLOOM_ANTHROPIC_MODEL", "claude-test")

	cfg := Load()

	if cfg.DBPath != "/env/test.db" {
		t.Errorf("DBPath = %q, want /env/test.db", cfg.DBPath)
	}
	if cfg.OllamaURL != "http://env:5678" {
		t.Errorf("OllamaURL = %q, want http://env:5678", cfg.OllamaURL)
	}
	if cfg.OllamaEmbedModel != "custom-model" {
		t.Errorf("OllamaEmbedModel = %q, want custom-model", cfg.OllamaEmbedModel)
	}
	if cfg.LLMProvider != "ollama" {
		t.Errorf("LLMProvider = %q, want ollama", cfg.LLMProvider)
	}
	if cfg.AnthropicAPIKey != "sk-test-key" {
		t.Errorf("AnthropicAPIKey = %q, want sk-test-key", cfg.AnthropicAPIKey)
	}
	if cfg.AnthropicModel != "claude-test" {
		t.Errorf("AnthropicModel = %q, want claude-test", cfg.AnthropicModel)
	}
}

func TestLoadEnvOverridesYAML(t *testing.T) {
	// Create config with one value
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	yaml := `ollama_url: http://yaml:1111`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)
	t.Setenv("REFLOOM_OLLAMA_URL", "http://env:2222")

	cfg := Load()

	// Env should override YAML
	if cfg.OllamaURL != "http://env:2222" {
		t.Errorf("OllamaURL = %q, want http://env:2222 (env should override yaml)", cfg.OllamaURL)
	}
}
