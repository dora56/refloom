package cli

import (
	"os"
	"path/filepath"
	"testing"
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
