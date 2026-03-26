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
	if cfg.EmbeddingBatchSize != 64 {
		t.Errorf("EmbeddingBatchSize = %d, want 64", cfg.EmbeddingBatchSize)
	}
	if !cfg.ExtractBatchWorkers.Auto {
		t.Errorf("ExtractBatchWorkers.Auto = false, want true")
	}
	if cfg.ExtractAutoMaxWorkers != 8 {
		t.Errorf("ExtractAutoMaxWorkers = %d, want 8", cfg.ExtractAutoMaxWorkers)
	}
	if cfg.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want 500", cfg.ChunkSize)
	}
	if cfg.Timeouts.Ingest != 30*time.Minute {
		t.Errorf("Timeouts.Ingest = %v, want 30m", cfg.Timeouts.Ingest)
	}
	if cfg.Timeouts.WorkerProbe != 2*time.Minute {
		t.Errorf("Timeouts.WorkerProbe = %v, want 2m", cfg.Timeouts.WorkerProbe)
	}
	if cfg.Timeouts.WorkerBatch != 5*time.Minute {
		t.Errorf("Timeouts.WorkerBatch = %v, want 5m", cfg.Timeouts.WorkerBatch)
	}
	if cfg.Timeouts.WorkerChunk != 3*time.Minute {
		t.Errorf("Timeouts.WorkerChunk = %v, want 3m", cfg.Timeouts.WorkerChunk)
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
embedding_batch_size: 64
extract_batch_workers: auto
extract_auto_max_workers: 6
chunk_size: 1000
timeouts:
  search: 10s
  worker_batch: 7m
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
	if cfg.EmbeddingBatchSize != 64 {
		t.Errorf("EmbeddingBatchSize = %d, want 64", cfg.EmbeddingBatchSize)
	}
	if !cfg.ExtractBatchWorkers.Auto {
		t.Errorf("ExtractBatchWorkers.Auto = false, want true")
	}
	if cfg.ExtractAutoMaxWorkers != 6 {
		t.Errorf("ExtractAutoMaxWorkers = %d, want 6", cfg.ExtractAutoMaxWorkers)
	}
	if cfg.ChunkSize != 1000 {
		t.Errorf("ChunkSize = %d, want 1000", cfg.ChunkSize)
	}
	if cfg.Timeouts.Search != 10*time.Second {
		t.Errorf("Timeouts.Search = %v, want 10s", cfg.Timeouts.Search)
	}
	if cfg.Timeouts.WorkerBatch != 7*time.Minute {
		t.Errorf("Timeouts.WorkerBatch = %v, want 7m", cfg.Timeouts.WorkerBatch)
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
	t.Setenv("REFLOOM_EMBEDDING_BATCH_SIZE", "16")
	t.Setenv("REFLOOM_EXTRACT_BATCH_WORKERS", "auto")
	t.Setenv("REFLOOM_EXTRACT_AUTO_MAX_WORKERS", "4")
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
	if cfg.EmbeddingBatchSize != 16 {
		t.Errorf("EmbeddingBatchSize = %d, want 16", cfg.EmbeddingBatchSize)
	}
	if !cfg.ExtractBatchWorkers.Auto {
		t.Errorf("ExtractBatchWorkers.Auto = false, want true")
	}
	if cfg.ExtractAutoMaxWorkers != 4 {
		t.Errorf("ExtractAutoMaxWorkers = %d, want 4", cfg.ExtractAutoMaxWorkers)
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

func TestLoadInvalidExtractBatchWorkersFallsBackToDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REFLOOM_EXTRACT_BATCH_WORKERS", "0")

	cfg := Load()

	if !cfg.ExtractBatchWorkers.Auto {
		t.Errorf("ExtractBatchWorkers.Auto = false, want default auto")
	}
}

func TestLoadExtractBatchWorkersAllowsMoreThanTwo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REFLOOM_EXTRACT_BATCH_WORKERS", "4")

	cfg := Load()

	if cfg.ExtractBatchWorkers.Auto || cfg.ExtractBatchWorkers.Fixed != 4 {
		t.Errorf("ExtractBatchWorkers = %+v, want fixed 4", cfg.ExtractBatchWorkers)
	}
}

func TestLoadExtractBatchWorkersFromYAMLFixedValue(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	yaml := `extract_batch_workers: 3`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	if cfg.ExtractBatchWorkers.Auto || cfg.ExtractBatchWorkers.Fixed != 3 {
		t.Errorf("ExtractBatchWorkers = %+v, want fixed 3", cfg.ExtractBatchWorkers)
	}
}

func TestLoadInvalidExtractAutoMaxWorkersFallsBackToDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("REFLOOM_EXTRACT_AUTO_MAX_WORKERS", "0")

	cfg := Load()

	if cfg.ExtractAutoMaxWorkers != 8 {
		t.Errorf("ExtractAutoMaxWorkers = %d, want default 8", cfg.ExtractAutoMaxWorkers)
	}
}

func TestLoadExtractAutoMaxWorkersFromYAML(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	yaml := `
extract_batch_workers: auto
extract_auto_max_workers: 5
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	if cfg.ExtractAutoMaxWorkers != 5 {
		t.Errorf("ExtractAutoMaxWorkers = %d, want 5", cfg.ExtractAutoMaxWorkers)
	}
}

func TestLoadInvalidYAMLReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Write invalid YAML
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(":::invalid yaml{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	// Should return defaults without panicking
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("OllamaURL = %q, want default http://localhost:11434", cfg.OllamaURL)
	}
	if cfg.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want default 500", cfg.ChunkSize)
	}
}

func TestLoadLegacyPythonPathCompatibility(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	yaml := `python_path: /tmp/python/refloom_worker/.venv/bin/python3`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	if cfg.PythonWorkerDir != "/tmp/python/refloom_worker/.venv/bin/python3" {
		t.Errorf("PythonWorkerDir = %q, want legacy python path", cfg.PythonWorkerDir)
	}
}

func TestLoadLegacyWorkerPerFileCompatibility(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".refloom")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatal(err)
	}
	yaml := `
timeouts:
  worker_per_file: 9m
`
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)

	cfg := Load()

	if cfg.Timeouts.WorkerProbe != 9*time.Minute {
		t.Errorf("WorkerProbe = %v, want 9m", cfg.Timeouts.WorkerProbe)
	}
	if cfg.Timeouts.WorkerBatch != 9*time.Minute {
		t.Errorf("WorkerBatch = %v, want 9m", cfg.Timeouts.WorkerBatch)
	}
	if cfg.Timeouts.WorkerChunk != 9*time.Minute {
		t.Errorf("WorkerChunk = %v, want 9m", cfg.Timeouts.WorkerChunk)
	}
}
