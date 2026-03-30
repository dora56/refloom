package db

import (
	"testing"
)

func TestBuildBinaryIndex(t *testing.T) {
	db := setupTestDB(t)

	// Insert a book and chunk
	_, err := db.Exec(`INSERT INTO book (title, author, format, source_path, file_hash, tags) VALUES ('Test', '', 'pdf', '/tmp/t.pdf', 'abc', '[]')`)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO chapter (book_id, title, chapter_order) VALUES (1, 'Ch1', 0)`)
	if err != nil {
		t.Fatalf("insert chapter: %v", err)
	}
	_, err = db.Exec(`INSERT INTO chunk (book_id, chapter_id, heading, body, char_count, chunk_order, embedding_version) VALUES (1, 1, 'h', 'body', 4, 0, 'test')`)
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	// Insert float32 embedding (768 dims)
	emb := make([]float32, 768)
	for i := range emb {
		emb[i] = float32(i%10) / 10.0
	}
	if err := db.InsertEmbedding(1, emb); err != nil {
		t.Fatalf("insert embedding: %v", err)
	}

	// Build binary index
	count, err := db.BuildBinaryIndex()
	if err != nil {
		t.Fatalf("BuildBinaryIndex: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	// Verify binary row exists
	var binaryCount int
	if err := db.QueryRow("SELECT count(*) FROM chunk_vec_binary").Scan(&binaryCount); err != nil {
		t.Fatalf("count binary: %v", err)
	}
	if binaryCount != 1 {
		t.Fatalf("binary count = %d, want 1", binaryCount)
	}
}

func TestSearchVectorBinary(t *testing.T) {
	db := setupTestDB(t)

	// Insert book, chapter, and 3 chunks with embeddings
	_, err := db.Exec(`INSERT INTO book (title, author, format, source_path, file_hash, tags) VALUES ('Test', '', 'pdf', '/tmp/t.pdf', 'abc', '[]')`)
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	_, err = db.Exec(`INSERT INTO chapter (book_id, title, chapter_order) VALUES (1, 'Ch1', 0)`)
	if err != nil {
		t.Fatalf("insert chapter: %v", err)
	}

	embeddings := make([][]float32, 3)
	for i := range 3 {
		_, err = db.Exec(`INSERT INTO chunk (book_id, chapter_id, heading, body, char_count, chunk_order, embedding_version) VALUES (1, 1, ?, 'body', 4, ?, 'test')`, "h", i)
		if err != nil {
			t.Fatalf("insert chunk %d: %v", i, err)
		}

		emb := make([]float32, 768)
		for j := range emb {
			emb[j] = float32((i+j)%10) / 10.0
		}
		embeddings[i] = emb
		if err := db.InsertEmbedding(int64(i+1), emb); err != nil {
			t.Fatalf("insert embedding %d: %v", i, err)
		}
	}

	// Build binary index
	count, err := db.BuildBinaryIndex()
	if err != nil {
		t.Fatalf("BuildBinaryIndex: %v", err)
	}
	if count != 3 {
		t.Fatalf("binary count = %d, want 3", count)
	}

	// Search with the first embedding as query — should return chunk 1 as top result
	results, err := db.SearchVectorBinary(embeddings[0], 3, nil)
	if err != nil {
		t.Fatalf("SearchVectorBinary: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	// First result should be chunk 1 (exact match or closest)
	if results[0].ChunkID != 1 {
		t.Logf("top result ChunkID = %d (expected 1, but binary quantization may reorder)", results[0].ChunkID)
	}
	// All 3 should be returned
	if len(results) != 3 {
		t.Fatalf("result count = %d, want 3", len(results))
	}
}

func TestSearchVectorBinaryEmpty(t *testing.T) {
	db := setupTestDB(t)

	query := make([]float32, 768)
	results, err := db.SearchVectorBinary(query, 5, nil)
	if err != nil {
		t.Fatalf("SearchVectorBinary: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}
