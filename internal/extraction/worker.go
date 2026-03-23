package extraction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

// Worker manages Python subprocess extraction.
type Worker struct {
	PythonPath string // Path to Python binary (e.g., python/.venv/bin/python3)
	WorkerDir  string // Directory containing the Python worker package
}

// NewWorker creates a new extraction worker.
// pythonPath: path to python3 binary
// workerDir: path to the directory containing the refloom_worker package
func NewWorker(pythonPath, workerDir string) *Worker {
	return &Worker{
		PythonPath: pythonPath,
		WorkerDir:  workerDir,
	}
}

// Extract runs the Python worker to extract and chunk a document.
func (w *Worker) Extract(ctx context.Context, bookPath, format string, chunkSize, chunkOverlap int) (*Response, error) {
	absPath, err := filepath.Abs(bookPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	req := Request{
		Command: "extract",
		Path:    absPath,
		Format:  format,
		Options: Options{
			ChunkSize:    chunkSize,
			ChunkOverlap: chunkOverlap,
		},
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	cmd := exec.CommandContext(ctx, w.PythonPath, "-m", "refloom_worker.main")
	cmd.Dir = w.WorkerDir
	cmd.Stdin = bytes.NewReader(reqJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("python worker failed: %w\nstderr: %s", err, stderr.String())
	}

	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w\nstdout: %s", err, stdout.String())
	}

	if resp.Status != "ok" {
		return nil, fmt.Errorf("extraction error: %s\n%s", resp.Error, resp.Details)
	}

	return &resp, nil
}
