package extraction

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"
)

// Verify PersistentWorker implements Extractor at compile time.
var _ Extractor = (*PersistentWorker)(nil)

// persistentProcess represents a single long-lived Python worker subprocess.
type persistentProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.ReadCloser
	mu     sync.Mutex // guards stdin writes and stdout reads
}

// PersistentWorker manages a pool of long-lived Python worker subprocesses.
// It implements the Extractor interface and reuses processes across commands.
type PersistentWorker struct {
	pythonPath string
	workerDir  string
	pool       chan *persistentProcess
	poolSize   int
	mu         sync.Mutex
}

// NewPersistentWorker creates a persistent worker pool with the given size.
// Call Close() when done to shut down all workers.
func NewPersistentWorker(pythonPath, workerDir string, poolSize int) (*PersistentWorker, error) {
	if poolSize < 1 {
		poolSize = 1
	}
	pw := &PersistentWorker{
		pythonPath: pythonPath,
		workerDir:  workerDir,
		pool:       make(chan *persistentProcess, poolSize),
		poolSize:   poolSize,
	}
	for range poolSize {
		proc, err := pw.spawnProcess()
		if err != nil {
			pw.Close()
			return nil, fmt.Errorf("spawn persistent worker: %w", err)
		}
		pw.pool <- proc
	}
	slog.Debug("persistent worker pool started", "size", poolSize)
	return pw, nil
}

func (pw *PersistentWorker) spawnProcess() (*persistentProcess, error) {
	cmd := exec.Command(pw.pythonPath, "-m", "refloom_worker.main", "--persistent") //nolint:gosec
	cmd.Dir = pw.workerDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()  //nolint:errcheck,gosec
		stdout.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)

	return &persistentProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
		stderr: stderr,
	}, nil
}

// sendCommand sends a JSON request to a worker and reads the JSON response.
func (pw *PersistentWorker) sendCommand(ctx context.Context, reqBody any, respBody any) error {
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Acquire a worker from the pool
	var proc *persistentProcess
	select {
	case proc = <-pw.pool:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Send request
	proc.mu.Lock()
	_, writeErr := proc.stdin.Write(append(reqJSON, '\n'))
	proc.mu.Unlock()

	if writeErr != nil {
		// Worker likely crashed; try to respawn
		slog.Warn("persistent worker write failed, respawning", "error", writeErr)
		pw.killProcess(proc)
		newProc, spawnErr := pw.spawnProcess()
		if spawnErr != nil {
			return fmt.Errorf("respawn failed: %w (original: %w)", spawnErr, writeErr)
		}
		pw.pool <- newProc
		return fmt.Errorf("worker write failed: %w", writeErr)
	}

	// Read response with context cancellation support
	responseCh := make(chan error, 1)
	go func() {
		proc.mu.Lock()
		defer proc.mu.Unlock()
		if !proc.stdout.Scan() {
			err := proc.stdout.Err()
			if err == nil {
				err = io.EOF
			}
			responseCh <- fmt.Errorf("worker read failed: %w", err)
			return
		}
		if err := json.Unmarshal(proc.stdout.Bytes(), respBody); err != nil {
			responseCh <- fmt.Errorf("parse response: %w", err)
			return
		}
		responseCh <- nil
	}()

	select {
	case err := <-responseCh:
		if err != nil {
			// Worker may be broken; respawn
			slog.Warn("persistent worker response error, respawning", "error", err)
			pw.killProcess(proc)
			newProc, spawnErr := pw.spawnProcess()
			if spawnErr != nil {
				return fmt.Errorf("respawn failed: %w (original: %w)", spawnErr, err)
			}
			pw.pool <- newProc
			return err
		}
		// Return worker to pool
		pw.pool <- proc
		return nil
	case <-ctx.Done():
		// Context cancelled; kill the worker since it may be mid-response
		pw.killProcess(proc)
		newProc, spawnErr := pw.spawnProcess()
		if spawnErr != nil {
			slog.Warn("respawn after cancel failed", "error", spawnErr)
		} else {
			pw.pool <- newProc
		}
		return ctx.Err()
	}
}

func (pw *PersistentWorker) killProcess(proc *persistentProcess) {
	proc.stdin.Close() //nolint:errcheck,gosec
	if proc.cmd.Process != nil {
		proc.cmd.Process.Kill() //nolint:errcheck,gosec
	}
	proc.cmd.Wait() //nolint:errcheck,gosec
	// Drain stderr for debugging
	if stderrBytes, err := io.ReadAll(proc.stderr); err == nil && len(stderrBytes) > 0 {
		slog.Debug("killed worker stderr", "output", string(stderrBytes))
	}
}

// Close shuts down all workers gracefully, waiting for in-flight commands to finish.
func (pw *PersistentWorker) Close() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Wait for all workers to return to the pool (blocks until in-flight commands finish)
	for range pw.poolSize {
		proc := <-pw.pool
		shutdownReq, _ := json.Marshal(map[string]string{"command": "shutdown"})
		proc.mu.Lock()
		proc.stdin.Write(append(shutdownReq, '\n')) //nolint:errcheck,gosec
		proc.stdin.Close()                          //nolint:errcheck,gosec
		proc.mu.Unlock()
		proc.cmd.Wait() //nolint:errcheck,gosec
	}
}

// Probe discovers extraction metadata for the given document.
func (pw *PersistentWorker) Probe(ctx context.Context, bookPath, format string) (*ProbeResponse, error) {
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
	if err := pw.sendCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("probe error: %s", resp.Error)
	}
	return &resp, nil
}

// ExtractPages extracts a bounded page range and writes the JSONL batch to disk.
func (pw *PersistentWorker) ExtractPages(ctx context.Context, req ExtractPagesRequest) (*ExtractPagesResponse, error) {
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
	if err := pw.sendCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("extract-pages error: %s", resp.Error)
	}
	return &resp, nil
}

// Chunk converts persisted pages into persisted chunks.
func (pw *PersistentWorker) Chunk(ctx context.Context, req ChunkRequest) (*ChunkResponse, error) {
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
	if err := pw.sendCommand(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("chunk error: %s", resp.Error)
	}
	return &resp, nil
}
