package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestEmitReindexEmbeddingProfile(t *testing.T) {
	t.Parallel()

	profile := reindexEmbeddingProfile{
		Model:     "nomic-embed-text",
		BatchSize: 32,
		Chunks:    120,
		Batches:   4,
		RequestMS: 1234,
		SaveMS:    456,
		TotalMS:   1900,
		Fails:     1,
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	if err := emitReindexEmbeddingProfile(profile); err != nil {
		t.Fatalf("emitReindexEmbeddingProfile: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("w.Close: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}

	var got reindexEmbeddingProfile
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got != profile {
		t.Fatalf("got %+v, want %+v", got, profile)
	}
}
