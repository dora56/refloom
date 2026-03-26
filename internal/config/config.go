package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type legacyTimeouts struct {
	WorkerPerFile time.Duration `yaml:"worker_per_file"`
}

type legacyConfig struct {
	PythonPath string         `yaml:"python_path"`
	Timeouts   legacyTimeouts `yaml:"timeouts"`
}

// Timeouts holds timeout configuration for each subsystem.
type Timeouts struct {
	Ingest      time.Duration `yaml:"ingest"`
	Search      time.Duration `yaml:"search"`
	Ask         time.Duration `yaml:"ask"`
	Reindex     time.Duration `yaml:"reindex"`
	WorkerProbe time.Duration `yaml:"worker_probe"`
	WorkerBatch time.Duration `yaml:"worker_batch"`
	WorkerChunk time.Duration `yaml:"worker_chunk"`
}

// Config holds all Refloom configuration.
type Config struct {
	DBPath                string                     `yaml:"db_path"`
	PythonWorkerDir       string                     `yaml:"python_worker_dir"`
	OllamaURL             string                     `yaml:"ollama_url"`
	OllamaEmbedModel      string                     `yaml:"ollama_embedding_model"`
	EmbeddingBatchSize    int                        `yaml:"embedding_batch_size"`
	EmbedParallelWorkers  int                        `yaml:"embed_parallel_workers"`
	ExtractBatchWorkers   ExtractBatchWorkersSetting `yaml:"extract_batch_workers"`
	ExtractAutoMaxWorkers int                        `yaml:"extract_auto_max_workers"`
	LLMProvider           string                     `yaml:"llm_provider"`
	AnthropicAPIKey       string                     `yaml:"anthropic_api_key"`
	AnthropicModel        string                     `yaml:"anthropic_model"`
	ChunkSize             int                        `yaml:"chunk_size"`
	ChunkOverlap          int                        `yaml:"chunk_overlap"`
	SearchLimit           int                        `yaml:"search_limit"`
	PromptBudget          int                        `yaml:"prompt_budget"`      // max total chars for LLM excerpt section
	PromptChunkLimit      int                        `yaml:"prompt_chunk_limit"` // max chars per chunk in LLM prompt
	Timeouts              Timeouts                   `yaml:"timeouts"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		DBPath:                "",
		PythonWorkerDir:       "",
		OllamaURL:             "http://localhost:11434",
		OllamaEmbedModel:      "nomic-embed-text",
		EmbeddingBatchSize:    64,
		EmbedParallelWorkers:  2,
		ExtractBatchWorkers:   DefaultExtractBatchWorkersSetting(),
		ExtractAutoMaxWorkers: DefaultExtractAutoMaxWorkers(),
		LLMProvider:           "claude-cli",
		AnthropicModel:        "",
		ChunkSize:             500,
		ChunkOverlap:          100,
		SearchLimit:           10,
		PromptBudget:          3000,
		PromptChunkLimit:      500,
		Timeouts: Timeouts{
			Ingest:      30 * time.Minute,
			Search:      30 * time.Second,
			Ask:         60 * time.Second,
			Reindex:     30 * time.Minute,
			WorkerProbe: 2 * time.Minute,
			WorkerBatch: 5 * time.Minute,
			WorkerChunk: 3 * time.Minute,
		},
	}
}

// Load reads config from ~/.refloom/config.yaml, merging with defaults and env vars.
func Load() *Config {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(home, ".refloom", "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil { //nolint:gosec
			if yamlErr := yaml.Unmarshal(data, cfg); yamlErr != nil {
				slog.Warn("config parse error, using defaults", "path", configPath, "error", yamlErr)
			}
			var legacy legacyConfig
			if legacyErr := yaml.Unmarshal(data, &legacy); legacyErr == nil {
				applyLegacyConfig(cfg, legacy)
			}
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
	if v := os.Getenv("REFLOOM_EMBEDDING_BATCH_SIZE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.EmbeddingBatchSize = parsed
		}
	}
	if v := os.Getenv("REFLOOM_EMBED_PARALLEL_WORKERS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.EmbedParallelWorkers = parsed
		}
	}
	if v := os.Getenv("REFLOOM_EXTRACT_BATCH_WORKERS"); v != "" {
		if parsed, err := ParseExtractBatchWorkersSetting(v); err == nil {
			cfg.ExtractBatchWorkers = parsed
		}
	}
	if v := os.Getenv("REFLOOM_EXTRACT_AUTO_MAX_WORKERS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.ExtractAutoMaxWorkers = parsed
		}
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

	if cfg.EmbeddingBatchSize <= 0 {
		cfg.EmbeddingBatchSize = DefaultConfig().EmbeddingBatchSize
	}
	if !cfg.ExtractBatchWorkers.Auto && cfg.ExtractBatchWorkers.Fixed <= 0 {
		cfg.ExtractBatchWorkers = DefaultConfig().ExtractBatchWorkers
	}
	if cfg.ExtractAutoMaxWorkers <= 0 {
		cfg.ExtractAutoMaxWorkers = DefaultConfig().ExtractAutoMaxWorkers
	}

	return cfg
}

func applyLegacyConfig(cfg *Config, legacy legacyConfig) {
	if cfg.PythonWorkerDir == "" && legacy.PythonPath != "" {
		cfg.PythonWorkerDir = legacy.PythonPath
	}

	if legacy.Timeouts.WorkerPerFile <= 0 {
		return
	}

	defaults := DefaultConfig().Timeouts
	if cfg.Timeouts.WorkerProbe == defaults.WorkerProbe {
		cfg.Timeouts.WorkerProbe = legacy.Timeouts.WorkerPerFile
	}
	if cfg.Timeouts.WorkerBatch == defaults.WorkerBatch {
		cfg.Timeouts.WorkerBatch = legacy.Timeouts.WorkerPerFile
	}
	if cfg.Timeouts.WorkerChunk == defaults.WorkerChunk {
		cfg.Timeouts.WorkerChunk = legacy.Timeouts.WorkerPerFile
	}
}
