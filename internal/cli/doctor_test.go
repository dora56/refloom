package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCountDirSizeEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files, sizeMB := countDirSize(dir)
	if files != 0 {
		t.Fatalf("files = %d, want 0", files)
	}
	if sizeMB != 0 {
		t.Fatalf("sizeMB = %f, want 0", sizeMB)
	}
}

func TestCountDirSizeNonExistent(t *testing.T) {
	t.Parallel()

	files, sizeMB := countDirSize("/nonexistent/path/xyz")
	if files != 0 {
		t.Fatalf("files = %d, want 0", files)
	}
	if sizeMB != 0 {
		t.Fatalf("sizeMB = %f, want 0", sizeMB)
	}
}

func TestCountDirSizeWithFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create 3 files with known sizes
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		data := make([]byte, 1024) // 1 KB each
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	files, sizeMB := countDirSize(dir)
	if files != 3 {
		t.Fatalf("files = %d, want 3", files)
	}
	// 3 * 1024 bytes = 3072 bytes = 0.00293 MB
	if sizeMB < 0.002 || sizeMB > 0.004 {
		t.Fatalf("sizeMB = %f, want ~0.003", sizeMB)
	}
}

func TestCountDirSizeSkipsSubdirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a file and a subdirectory with a file
	if err := os.WriteFile(filepath.Join(dir, "root.json"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "nested.json"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	// countDirSize walks recursively, so it should count both files
	files, _ := countDirSize(dir)
	if files != 2 {
		t.Fatalf("files = %d, want 2 (root + nested)", files)
	}
}

func TestCheckDBWithValidDB(t *testing.T) {
	setupCfgForTest(t)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	cfg.DBPath = dbPath

	result := checkDB()
	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok; Detail = %s", result.Status, result.Detail)
	}
	if !strings.Contains(result.Detail, "0 books") {
		t.Fatalf("Detail = %q, want '0 books'", result.Detail)
	}
}

func TestCheckDBWithInvalidPath(t *testing.T) {
	setupCfgForTest(t)
	cfg.DBPath = "/nonexistent/dir/that/wont/work/test.db"

	result := checkDB()
	if result.Status != "fail" {
		t.Fatalf("Status = %q, want fail", result.Status)
	}
}

func TestCheckDiskWithCacheFiles(t *testing.T) {
	setupCfgForTest(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a fake DB file
	dbDir := filepath.Join(home, ".refloom")
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "refloom.db")
	if err := os.WriteFile(dbPath, make([]byte, 1024), 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}
	cfg.DBPath = dbPath

	// Create OCR cache files
	cacheDir := filepath.Join(home, ".refloom", "cache", "ocr")
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "abc.json"), []byte(`{"text":"hello"}`), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	result := checkDisk()
	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok", result.Status)
	}
	if !strings.Contains(result.Detail, "OCR cache:") || strings.Contains(result.Detail, "none") {
		t.Fatalf("Detail = %q, should mention OCR cache with files", result.Detail)
	}
	if !strings.Contains(result.Detail, "1 files") {
		t.Fatalf("Detail = %q, want '1 files'", result.Detail)
	}
}

func TestCheckDiskNoCacheDir(t *testing.T) {
	setupCfgForTest(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	dbDir := filepath.Join(home, ".refloom")
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dbDir, "refloom.db")
	if err := os.WriteFile(dbPath, make([]byte, 512), 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}
	cfg.DBPath = dbPath

	result := checkDisk()
	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok", result.Status)
	}
	if !strings.Contains(result.Detail, "OCR cache: none") {
		t.Fatalf("Detail = %q, want 'OCR cache: none'", result.Detail)
	}
}
