package cli

import (
	"os"
	"path/filepath"
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
