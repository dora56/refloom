package extraction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
)

// Worker manages Python subprocess extraction.
type Worker struct {
	PythonPath string
	WorkerDir  string
}

// NewWorker creates a new extraction worker.
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

	slog.Debug("spawning python worker", "python", w.PythonPath, "dir", w.WorkerDir, "path", absPath, "format", format)

	cmd := exec.CommandContext(ctx, w.PythonPath, "-m", "refloom_worker.main")
	cmd.Dir = w.WorkerDir
	cmd.Stdin = bytes.NewReader(reqJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			slog.Error("python worker stderr", "output", stderrStr)
		}
		return nil, fmt.Errorf("python worker failed: %w", err)
	}

	if stderrStr := stderr.String(); stderrStr != "" {
		slog.Debug("python worker stderr", "output", stderrStr)
	}

	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w (stdout length: %d bytes)", err, stdout.Len())
	}

	if resp.Status != "ok" {
		return nil, fmt.Errorf("extraction error: %s", resp.Error)
	}

	return &resp, nil
}
