package extraction

import (
	"encoding/json"
	"testing"
)

func TestRequestMarshal(t *testing.T) {
	req := Request{
		Command: "extract",
		Path:    "/tmp/test.pdf",
		Format:  "pdf",
		Options: Options{ChunkSize: 500, ChunkOverlap: 100},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Command != "extract" {
		t.Errorf("Command = %q, want extract", got.Command)
	}
	if got.Path != "/tmp/test.pdf" {
		t.Errorf("Path = %q, want /tmp/test.pdf", got.Path)
	}
	if got.Options.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want 500", got.Options.ChunkSize)
	}
}

func TestResponseUnmarshal(t *testing.T) {
	raw := `{
		"status": "ok",
		"quality": "text_corrupt",
		"book": {"title": "Test Book", "author": "Author", "format": "pdf", "page_count": 100},
		"chapters": [
			{"title": "Chapter 1", "order": 0, "page_start": 1, "page_end": 50}
		],
		"chunks": [
			{"chapter_order": 0, "heading": "Intro", "body": "Hello world", "char_count": 11, "page_start": 1, "page_end": 5, "chunk_order": 0}
		]
	}`

	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("Status = %q, want ok", resp.Status)
	}
	if resp.Quality != "text_corrupt" {
		t.Errorf("Quality = %q, want text_corrupt", resp.Quality)
	}
	if resp.Book.Title != "Test Book" {
		t.Errorf("Book.Title = %q, want Test Book", resp.Book.Title)
	}
	if resp.Book.PageCount != 100 {
		t.Errorf("Book.PageCount = %d, want 100", resp.Book.PageCount)
	}
	if len(resp.Chapters) != 1 {
		t.Fatalf("Chapters len = %d, want 1", len(resp.Chapters))
	}
	if resp.Chapters[0].Title != "Chapter 1" {
		t.Errorf("Chapter title = %q, want Chapter 1", resp.Chapters[0].Title)
	}
	if resp.Chapters[0].PageStart == nil || *resp.Chapters[0].PageStart != 1 {
		t.Errorf("Chapter PageStart = %v, want 1", resp.Chapters[0].PageStart)
	}
	if len(resp.Chunks) != 1 {
		t.Fatalf("Chunks len = %d, want 1", len(resp.Chunks))
	}
	if resp.Chunks[0].Body != "Hello world" {
		t.Errorf("Chunk body = %q, want Hello world", resp.Chunks[0].Body)
	}
}

func TestResponseUnmarshalError(t *testing.T) {
	raw := `{"status": "error", "error": "unsupported format", "details": "xyz"}`

	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("Status = %q, want error", resp.Status)
	}
	if resp.Error != "unsupported format" {
		t.Errorf("Error = %q, want unsupported format", resp.Error)
	}
}

func TestResponseUnmarshalNullPages(t *testing.T) {
	raw := `{
		"status": "ok",
		"book": {"title": "EPUB", "format": "epub", "page_count": 0},
		"chapters": [{"title": "Ch1", "order": 0, "page_start": null, "page_end": null}],
		"chunks": [{"chapter_order": 0, "heading": "", "body": "text", "char_count": 4, "page_start": null, "page_end": null, "chunk_order": 0}]
	}`

	var resp Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Chapters[0].PageStart != nil {
		t.Error("PageStart should be nil for null JSON")
	}
	if resp.Chunks[0].PageStart != nil {
		t.Error("Chunk PageStart should be nil for null JSON")
	}
}
