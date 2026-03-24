package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Timeouts holds timeout configuration for each subsystem.
type Timeouts struct {
	Ingest        time.Duration `yaml:"ingest"`
	Search        time.Duration `yaml:"search"`
	Ask           time.Duration `yaml:"ask"`
	Reindex       time.Duration `yaml:"reindex"`
	WorkerPerFile time.Duration `yaml:"worker_per_file"`
}

// Config holds all Refloom configuration.
type Config struct {
	DBPath           string   `yaml:"db_path"`
	PythonWorkerDir  string   `yaml:"python_worker_dir"`
	OllamaURL        string   `yaml:"ollama_url"`
	OllamaEmbedModel string   `yaml:"ollama_embedding_model"`
	LLMProvider      string   `yaml:"llm_provider"`
	AnthropicAPIKey  string   `yaml:"anthropic_api_key"`
	AnthropicModel   string   `yaml:"anthropic_model"`
	ChunkSize        int      `yaml:"chunk_size"`
	ChunkOverlap     int      `yaml:"chunk_overlap"`
	SearchLimit      int      `yaml:"search_limit"`
	Timeouts         Timeouts `yaml:"timeouts"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DBPath:           "",
		PythonWorkerDir:  "",
		OllamaURL:        "http://localhost:11434",
		OllamaEmbedModel: "nomic-embed-text",
		LLMProvider:      "claude-cli",
		AnthropicModel:   "",
		ChunkSize:        500,
		ChunkOverlap:     100,
		SearchLimit:      10,
		Timeouts: Timeouts{
			Ingest:        30 * time.Minute,
			Search:        30 * time.Second,
			Ask:           60 * time.Second,
			Reindex:       30 * time.Minute,
			WorkerPerFile: 5 * time.Minute,
		},
	}
}

// Load reads config from ~/.refloom/config.yaml, merging with defaults and env vars.
func Load() *Config {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".refloom", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			yaml.Unmarshal(data, cfg)
		}
	}

	if v := os.Getenv("REFLOOM_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("REFLOOM_WORKER_DIR"); v != "" {
		cfg.PythonWorkerDir = v
	}
	if v := os.Getenv("REFLOOM_OLLAMA_URL"); v != "" {
		cfg.OllamaURL = v
	}
	if v := os.Getenv("REFLOOM_EMBEDDING_MODEL"); v != "" {
		cfg.OllamaEmbedModel = v
	}
	if v := os.Getenv("REFLOOM_LLM_PROVIDER"); v != "" {
		cfg.LLMProvider = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.AnthropicAPIKey = v
	}
	if v := os.Getenv("REFLOOM_ANTHROPIC_MODEL"); v != "" {
		cfg.AnthropicModel = v
	}

	return cfg
}
