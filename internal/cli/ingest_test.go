package cli

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/dora56/refloom/internal/extraction"
)

func TestShouldLogEmbeddingProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		previous int
		current  int
		want     bool
	}{
		{
			name:     "before first threshold",
			previous: 0,
			current:  32,
			want:     false,
		},
		{
			name:     "crosses first threshold within a batch",
			previous: 96,
			current:  128,
			want:     true,
		},
		{
			name:     "same threshold does not log twice",
			previous: 128,
			current:  160,
			want:     false,
		},
		{
			name:     "crosses later threshold within a batch",
			previous: 192,
			current:  224,
			want:     true,
		},
		{
			name:     "non-increasing progress does not log",
			previous: 224,
			current:  224,
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := shouldLogEmbeddingProgress(tc.previous, tc.current)
			if got != tc.want {
				t.Fatalf("shouldLogEmbeddingProgress(%d, %d) = %v, want %v", tc.previous, tc.current, got, tc.want)
			}
		})
	}
}

func TestResolvedEmbeddingBatchSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured int
		want       int
	}{
		{name: "uses configured size", configured: 64, want: 64},
		{name: "falls back for zero", configured: 0, want: embeddingBatchSize},
		{name: "falls back for negative", configured: -1, want: embeddingBatchSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := resolvedEmbeddingBatchSize(tc.configured); got != tc.want {
				t.Fatalf("resolvedEmbeddingBatchSize(%d) = %d, want %d", tc.configured, got, tc.want)
			}
		})
	}
}

func TestResolveConfiguredWorkerDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workerRoot := filepath.Join(root, "python")
	packageDir := filepath.Join(workerRoot, "refloom_worker")
	venvPython := filepath.Join(packageDir, ".venv", "bin", "python3")

	if err := os.MkdirAll(filepath.Dir(venvPython), 0o750); err != nil {
		t.Fatalf("mkdir venv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "main.py"), []byte(""), 0o600); err != nil {
		t.Fatalf("write main.py: %v", err)
	}
	if err := os.WriteFile(venvPython, []byte(""), 0o600); err != nil {
		t.Fatalf("write python3: %v", err)
	}

	tests := []struct {
		name       string
		configured string
		wantDir    string
		wantPython string
	}{
		{
			name:       "accepts worker root",
			configured: workerRoot,
			wantDir:    workerRoot,
			wantPython: venvPython,
		},
		{
			name:       "normalizes package dir to worker root",
			configured: packageDir,
			wantDir:    workerRoot,
			wantPython: venvPython,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotDir, gotPython := resolveConfiguredWorkerDir(tc.configured)
			if gotDir != tc.wantDir {
				t.Fatalf("resolveConfiguredWorkerDir(%q) dir = %q, want %q", tc.configured, gotDir, tc.wantDir)
			}
			if gotPython != tc.wantPython {
				t.Fatalf("resolveConfiguredWorkerDir(%q) python = %q, want %q", tc.configured, gotPython, tc.wantPython)
			}
		})
	}
}

func TestShouldWarnEmbeddingFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		fails int
		total int
		want  bool
	}{
		{name: "no failures", fails: 0, total: 100, want: false},
		{name: "49 percent", fails: 49, total: 100, want: false},
		{name: "50 percent", fails: 50, total: 100, want: false},
		{name: "51 percent", fails: 51, total: 100, want: true},
		{name: "all failed", fails: 10, total: 10, want: true},
		{name: "zero total", fails: 0, total: 0, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldWarnEmbeddingFailures(tc.fails, tc.total); got != tc.want {
				t.Fatalf("shouldWarnEmbeddingFailures(%d, %d) = %v, want %v", tc.fails, tc.total, got, tc.want)
			}
		})
	}
}

func TestApplyEmbeddingSkippedProfile(t *testing.T) {
	t.Parallel()

	profile := &ingestProfile{
		EmbedMS:        99,
		EmbedBatchSize: 32,
		EmbedBatches:   3,
	}

	applyEmbeddingSkippedProfile(profile)

	if !profile.EmbedSkipped {
		t.Fatalf("EmbedSkipped = false, want true")
	}
	if profile.EmbedMS != 0 {
		t.Fatalf("EmbedMS = %d, want 0", profile.EmbedMS)
	}
	if profile.EmbedBatches != 0 {
		t.Fatalf("EmbedBatches = %d, want 0", profile.EmbedBatches)
	}
}

func newMockOllamaServer(t *testing.T, dim int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input any `json:"input"`
		}
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck,gosec

		// Determine count from input
		count := 1
		if arr, ok := req.Input.([]any); ok {
			count = len(arr)
		}

		embeddings := make([][]float64, count)
		for i := range count {
			vec := make([]float64, dim)
			vec[0] = float64(i)
			embeddings[i] = vec
		}
		json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings}) //nolint:errcheck,gosec
	}))
}

func TestSaveChunkEmbeddingsParallelProducesSameResultAsSequential(t *testing.T) {
	// Setup: mock Ollama, in-memory DB, cfg with parallel workers
	server := newMockOllamaServer(t, 768)
	defer server.Close()

	setupCfgForTest(t)
	cfg.EmbedParallelWorkers = 2

	database := openTestDB(t)

	client := &embedding.Client{
		BaseURL:    server.URL,
		Model:      "test",
		MaxRetries: 1,
		HTTPClient: server.Client(),
	}

	chunks := make([]embeddedChunk, 10)
	for i := range chunks {
		chunks[i] = embeddedChunk{ChunkID: int64(i + 1), Body: "test text"}
	}

	log := slog.Default()
	stats, err := saveChunkEmbeddings(context.Background(), database, client, "test", 3, chunks, false, log)
	if err != nil {
		t.Fatalf("saveChunkEmbeddings: %v", err)
	}
	if stats.Fails != 0 {
		t.Fatalf("Fails = %d, want 0", stats.Fails)
	}
	if stats.Batches != 4 { // 10 chunks / 3 batch size = 4 batches
		t.Fatalf("Batches = %d, want 4", stats.Batches)
	}
}

func TestSaveChunkEmbeddingsSequentialFallback(t *testing.T) {
	server := newMockOllamaServer(t, 768)
	defer server.Close()

	setupCfgForTest(t)
	cfg.EmbedParallelWorkers = 1 // force sequential

	database := openTestDB(t)

	client := &embedding.Client{
		BaseURL:    server.URL,
		Model:      "test",
		MaxRetries: 1,
		HTTPClient: server.Client(),
	}

	chunks := []embeddedChunk{
		{ChunkID: 1, Body: "hello"},
		{ChunkID: 2, Body: "world"},
	}

	log := slog.Default()
	stats, err := saveChunkEmbeddings(context.Background(), database, client, "test", 64, chunks, false, log)
	if err != nil {
		t.Fatalf("saveChunkEmbeddings: %v", err)
	}
	if stats.Fails != 0 {
		t.Fatalf("Fails = %d, want 0", stats.Fails)
	}
	if stats.Batches != 1 {
		t.Fatalf("Batches = %d, want 1", stats.Batches)
	}
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() }) //nolint:errcheck,gosec
	return database
}

func TestApplyExtractResultToProfile(t *testing.T) {
	t.Parallel()

	resp := &stagedExtractResult{
		ProbeMS:                 10,
		PageExtractMS:           200,
		PageExtractSumMS:        180,
		ChunkMS:                 30,
		BatchCount:              3,
		FailedBatchCount:        1,
		ExtractWorkersMode:      "auto",
		ExtractWorkersRequested: "auto",
		ExtractWorkersUsed:      4,
		ExtractAutoMaxWorkers:   8,
		ExtractAutoEffectiveCap: 4,
		ExtractAutoTier:         "pro",
		ExtractAutoCandidates:   []int{1, 2, 4},
		AutoWorkerReason:        "warm-up",
		Resumed:                 true,
		JobDir:                  "/tmp/test-job",
		Quality:                 "ok",
		Chapters:                []extraction.ChapterInfo{{Title: "ch1"}, {Title: "ch2"}},
		Chunks:                  []extraction.ChunkInfo{{Body: "a"}, {Body: "b"}, {Body: "c"}},
		Stats: extraction.Stats{
			OCRPages:      10,
			OCRRetries:    2,
			OCRMS:         5000,
			OCRFastPages:  8,
			OCRRetryPages: 2,
		},
	}

	profile := &ingestProfile{}
	applyExtractResultToProfile(profile, resp)

	if profile.ExtractMS != 240 {
		t.Fatalf("ExtractMS = %d, want 240", profile.ExtractMS)
	}
	if profile.Chapters != 2 {
		t.Fatalf("Chapters = %d, want 2", profile.Chapters)
	}
	if profile.Chunks != 0 {
		t.Fatalf("Chunks = %d, want 0 (set later by persistBookData)", profile.Chunks)
	}
	if profile.Quality != "ok" {
		t.Fatalf("Quality = %q, want ok", profile.Quality)
	}
	if !profile.ParallelExtractEnabled {
		t.Fatal("ParallelExtractEnabled = false, want true")
	}
	if profile.OCRFastPages != 8 {
		t.Fatalf("OCRFastPages = %d, want 8", profile.OCRFastPages)
	}
}

func TestApplyExtractResultToProfileDefaultQuality(t *testing.T) {
	t.Parallel()

	resp := &stagedExtractResult{Quality: ""}
	profile := &ingestProfile{}
	applyExtractResultToProfile(profile, resp)

	if profile.Quality != "ok" {
		t.Fatalf("Quality = %q, want ok", profile.Quality)
	}
}

func TestHandleExtractionQuality(t *testing.T) {
	t.Parallel()

	database := openTestDB(t)

	tests := []struct {
		name    string
		quality string
		skip    bool
	}{
		{name: "ok passes", quality: "ok", skip: false},
		{name: "ocr_required skips", quality: "ocr_required", skip: true},
		{name: "extract_failed skips", quality: "extract_failed", skip: true},
		{name: "text_corrupt proceeds", quality: "text_corrupt", skip: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			skip, err := handleExtractionQuality(tc.quality, "/tmp/test.pdf", database)
			if err != nil {
				t.Fatalf("handleExtractionQuality: %v", err)
			}
			if skip != tc.skip {
				t.Fatalf("skip = %v, want %v", skip, tc.skip)
			}
		})
	}
}

// setupCfgForTest is defined in extract_job_test.go
