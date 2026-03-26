package extraction

import (
	"encoding/json"
	"testing"
)

func TestProbeRequestMarshal(t *testing.T) {
	t.Parallel()

	req := ProbeRequest{
		Command: "probe",
		Path:    "/tmp/test.pdf",
		Format:  "pdf",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ProbeRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Command != "probe" {
		t.Fatalf("Command = %q, want probe", got.Command)
	}
	if got.Path != "/tmp/test.pdf" {
		t.Fatalf("Path = %q, want /tmp/test.pdf", got.Path)
	}
}

func TestProbeResponseUnmarshal(t *testing.T) {
	t.Parallel()

	raw := `{
		"status": "ok",
		"book": {"title": "Test Book", "author": "Author", "format": "pdf", "page_count": 100},
		"chapters": [{"title": "Chapter 1", "order": 0, "page_start": 1, "page_end": 50}],
		"extraction_mode": "ocr-heavy",
		"recommended_batch_size": 16,
		"ocr_candidate_pages_estimate": 2
	}`

	var resp ProbeResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Book.Title != "Test Book" {
		t.Fatalf("Book.Title = %q, want Test Book", resp.Book.Title)
	}
	if resp.ExtractionMode != "ocr-heavy" {
		t.Fatalf("ExtractionMode = %q, want ocr-heavy", resp.ExtractionMode)
	}
	if resp.RecommendedBatchSize != 16 {
		t.Fatalf("RecommendedBatchSize = %d, want 16", resp.RecommendedBatchSize)
	}
}

func TestExtractPagesResponseUnmarshal(t *testing.T) {
	t.Parallel()

	raw := `{
		"status": "ok",
		"pages_written": 32,
		"stats": {
			"ocr_pages": 3,
			"ocr_retries": 1,
			"ocr_ms": 1200,
			"ocr_fast_pages": 3,
			"ocr_retry_pages": 1
		},
		"batch_ms": 2450
	}`

	var resp ExtractPagesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.PagesWritten != 32 {
		t.Fatalf("PagesWritten = %d, want 32", resp.PagesWritten)
	}
	if resp.Stats.OCRPages != 3 {
		t.Fatalf("Stats.OCRPages = %d, want 3", resp.Stats.OCRPages)
	}
	if resp.BatchMS != 2450 {
		t.Fatalf("BatchMS = %d, want 2450", resp.BatchMS)
	}
}

func TestChunkResponseUnmarshal(t *testing.T) {
	t.Parallel()

	raw := `{
		"status": "ok",
		"quality": "text_corrupt",
		"chunks_written": 12,
		"chunk_ms": 991
	}`

	var resp ChunkResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Quality != "text_corrupt" {
		t.Fatalf("Quality = %q, want text_corrupt", resp.Quality)
	}
	if resp.ChunksWritten != 12 {
		t.Fatalf("ChunksWritten = %d, want 12", resp.ChunksWritten)
	}
}
