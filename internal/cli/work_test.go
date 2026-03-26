package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneExtractWorkdirsDryRunOnlyRemovesCompleted(t *testing.T) {
	t.Parallel()

	workRoot := t.TempDir()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	completedDir := makeManifestDir(t, workRoot, "completed-job", extractJobManifest{Status: "completed"}, now.Add(-200*time.Hour))
	failedDir := makeManifestDir(t, workRoot, "failed-job", extractJobManifest{Status: "failed"}, now.Add(-200*time.Hour))
	resumableDir := makeManifestDir(t, workRoot, "resumable-job", extractJobManifest{
		Status:    "extracting_pages",
		Completed: []extractCompletedBatch{{PageStart: 1, PageEnd: 16}},
	}, now.Add(-200*time.Hour))

	result, err := pruneExtractWorkdirs(workRoot, now, 168*time.Hour, false, true)
	if err != nil {
		t.Fatalf("pruneExtractWorkdirs: %v", err)
	}

	if result.RemovedJobs != 1 {
		t.Fatalf("RemovedJobs = %d, want 1", result.RemovedJobs)
	}
	if result.KeptFailedJobs != 1 {
		t.Fatalf("KeptFailedJobs = %d, want 1", result.KeptFailedJobs)
	}
	if result.ProtectedJobs != 1 {
		t.Fatalf("ProtectedJobs = %d, want 1", result.ProtectedJobs)
	}
	assertDirExists(t, completedDir)
	assertDirExists(t, failedDir)
	assertDirExists(t, resumableDir)
}

func TestPruneExtractWorkdirsRemovesFailedWhenIncluded(t *testing.T) {
	t.Parallel()

	workRoot := t.TempDir()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	completedDir := makeManifestDir(t, workRoot, "completed-job", extractJobManifest{Status: "completed"}, now.Add(-200*time.Hour))
	failedDir := makeManifestDir(t, workRoot, "failed-job", extractJobManifest{Status: "failed"}, now.Add(-200*time.Hour))
	resumableDir := makeManifestDir(t, workRoot, "resumable-job", extractJobManifest{
		Status:    "chunking",
		Completed: []extractCompletedBatch{{PageStart: 1, PageEnd: 16}},
	}, now.Add(-200*time.Hour))

	result, err := pruneExtractWorkdirs(workRoot, now, 168*time.Hour, true, false)
	if err != nil {
		t.Fatalf("pruneExtractWorkdirs: %v", err)
	}

	if result.RemovedJobs != 2 {
		t.Fatalf("RemovedJobs = %d, want 2", result.RemovedJobs)
	}
	assertDirMissing(t, completedDir)
	assertDirMissing(t, failedDir)
	assertDirExists(t, resumableDir)
}

func TestPruneExtractWorkdirsSkipsRecentJobs(t *testing.T) {
	t.Parallel()

	workRoot := t.TempDir()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	recentDir := makeManifestDir(t, workRoot, "recent-job", extractJobManifest{Status: "completed"}, now.Add(-2*time.Hour))

	result, err := pruneExtractWorkdirs(workRoot, now, 168*time.Hour, false, false)
	if err != nil {
		t.Fatalf("pruneExtractWorkdirs: %v", err)
	}

	if result.RemovedJobs != 0 {
		t.Fatalf("RemovedJobs = %d, want 0", result.RemovedJobs)
	}
	assertDirExists(t, recentDir)
}

func makeManifestDir(t *testing.T, workRoot, name string, manifest extractJobManifest, modTime time.Time) string {
	t.Helper()

	jobDir := filepath.Join(workRoot, name)
	if err := os.MkdirAll(filepath.Join(jobDir, "pages"), 0o750); err != nil {
		t.Fatalf("mkdir pages: %v", err)
	}
	if manifest.JobID == "" {
		manifest.JobID = name
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath := filepath.Join(jobDir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	payloadPath := filepath.Join(jobDir, "payload.txt")
	if err := os.WriteFile(payloadPath, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := os.Chtimes(manifestPath, modTime, modTime); err != nil {
		t.Fatalf("chtimes manifest: %v", err)
	}
	if err := os.Chtimes(payloadPath, modTime, modTime); err != nil {
		t.Fatalf("chtimes payload: %v", err)
	}
	return jobDir
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertDirMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat err=%v", path, err)
	}
}
