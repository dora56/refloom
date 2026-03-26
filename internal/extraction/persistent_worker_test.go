package extraction

import (
	"bufio"
	"context"
	"os/exec"
	"testing"
	"time"
)

func findPython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("python3 not found in PATH")
	return ""
}

// newTestWorker creates a PersistentWorker using a test echo script.
func newTestWorker(t *testing.T, poolSize int) *PersistentWorker {
	t.Helper()
	python := findPython(t)
	// Override spawnProcess to use the echo script directly
	pw := &PersistentWorker{
		pythonPath: python,
		workerDir:  ".",
		pool:       make(chan *persistentProcess, poolSize),
		poolSize:   poolSize,
	}
	for range poolSize {
		cmd := exec.Command(python, "testdata/echo_worker.py") //nolint:gosec
		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			t.Fatalf("start echo worker: %v", err)
		}
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024), 16*1024*1024)
		pw.pool <- &persistentProcess{cmd: cmd, stdin: stdin, stdout: scanner, stderr: stderr}
	}
	return pw
}

func TestPersistentWorkerSendMultipleCommands(t *testing.T) {
	pw := newTestWorker(t, 1)
	defer pw.Close()

	ctx := context.Background()
	for i := range 3 {
		var resp map[string]any
		req := map[string]any{"command": "echo", "index": i}
		if err := pw.sendCommand(ctx, req, &resp); err != nil {
			t.Fatalf("sendCommand %d: %v", i, err)
		}
		if resp["status"] != "ok" {
			t.Fatalf("response %d status = %v, want ok", i, resp["status"])
		}
		if int(resp["index"].(float64)) != i {
			t.Fatalf("response %d index = %v, want %d", i, resp["index"], i)
		}
	}
}

func TestPersistentWorkerPoolSizeMultiple(t *testing.T) {
	pw := newTestWorker(t, 3)
	defer pw.Close()

	ctx := context.Background()
	for i := range 5 {
		var resp map[string]any
		req := map[string]any{"command": "echo", "n": i}
		if err := pw.sendCommand(ctx, req, &resp); err != nil {
			t.Fatalf("sendCommand %d: %v", i, err)
		}
	}
}

func TestPersistentWorkerCloseIsGraceful(t *testing.T) {
	pw := newTestWorker(t, 2)

	// Send one command to verify it works
	ctx := context.Background()
	var resp map[string]any
	if err := pw.sendCommand(ctx, map[string]any{"command": "echo"}, &resp); err != nil {
		t.Fatalf("sendCommand: %v", err)
	}

	// Close should not hang
	done := make(chan struct{})
	go func() {
		pw.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() timed out")
	}
}

func TestPersistentWorkerContextCancel(t *testing.T) {
	pw := newTestWorker(t, 1)
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	var resp map[string]any
	err := pw.sendCommand(ctx, map[string]any{"command": "echo"}, &resp)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
