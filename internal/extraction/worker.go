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

// Extractor defines the extraction operations used by the ingest pipeline.
type Extractor interface {
	Probe(ctx context.Context, bookPath, format string) (*ProbeResponse, error)
	ExtractPages(ctx context.Context, req ExtractPagesRequest) (*ExtractPagesResponse, error)
	Chunk(ctx context.Context, req ChunkRequest) (*ChunkResponse, error)
}

// Verify Worker implements Extractor at compile time.
var _ Extractor = (*Worker)(nil)

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

// Probe discovers extraction metadata for the given document.
func (w *Worker) Probe(ctx context.Context, bookPath, format string) (*ProbeResponse, error) {
	absPath, err := filepath.Abs(bookPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	req := ProbeRequest{
		Command: "probe",
		Path:    absPath,
		Format:  format,
	}

	var resp ProbeResponse
	if err := w.runJSONCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("probe error: %s", resp.Error)
	}
	return &resp, nil
}

// ExtractPages extracts a bounded page range and writes the JSONL batch to disk.
func (w *Worker) ExtractPages(ctx context.Context, req ExtractPagesRequest) (*ExtractPagesResponse, error) {
	req.Command = "extract-pages"
	if absPath, err := filepath.Abs(req.Path); err == nil {
		req.Path = absPath
	} else {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if absOut, err := filepath.Abs(req.OutputPath); err == nil {
		req.OutputPath = absOut
	} else {
		return nil, fmt.Errorf("resolve output path: %w", err)
	}

	var resp ExtractPagesResponse
	if err := w.runJSONCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("extract-pages error: %s", resp.Error)
	}
	return &resp, nil
}

// Chunk converts persisted pages into persisted chunks.
func (w *Worker) Chunk(ctx context.Context, req ChunkRequest) (*ChunkResponse, error) {
	req.Command = "chunk"
	if absPages, err := filepath.Abs(req.PagesPath); err == nil {
		req.PagesPath = absPages
	} else {
		return nil, fmt.Errorf("resolve pages path: %w", err)
	}
	if absChapters, err := filepath.Abs(req.ChaptersPath); err == nil {
		req.ChaptersPath = absChapters
	} else {
		return nil, fmt.Errorf("resolve chapters path: %w", err)
	}
	if absOut, err := filepath.Abs(req.OutputPath); err == nil {
		req.OutputPath = absOut
	} else {
		return nil, fmt.Errorf("resolve output path: %w", err)
	}

	var resp ChunkResponse
	if err := w.runJSONCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("chunk error: %s", resp.Error)
	}
	return &resp, nil
}

func (w *Worker) runJSONCommand(ctx context.Context, reqBody any, respBody any) error {
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	slog.Debug("spawning python worker", "python", w.PythonPath, "dir", w.WorkerDir)

	cmd := exec.CommandContext(ctx, w.PythonPath, "-m", "refloom_worker.main") //nolint:gosec
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
		return fmt.Errorf("python worker failed: %w", err)
	}

	if stderrStr := stderr.String(); stderrStr != "" {
		slog.Debug("python worker stderr", "output", stderrStr)
	}

	if err := json.Unmarshal(stdout.Bytes(), respBody); err != nil {
		return fmt.Errorf("parse response: %w (stdout length: %d bytes)", err, stdout.Len())
	}
	return nil
}
