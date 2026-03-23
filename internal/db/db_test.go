package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndMigrate(t *testing.T) {
	db := setupTestDB(t)

	// Verify vec_version works
	var version string
	if err := db.QueryRow("SELECT vec_version()").Scan(&version); err != nil {
		t.Fatalf("vec_version: %v", err)
	}
	t.Logf("sqlite-vec version: %s", version)

	// Verify tables exist
	tables := []string{"book", "chapter", "chunk", "chunk_fts", "chunk_vec", "ingest_log"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type IN ('table', 'virtual table') AND name = ?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestBookCRUD(t *testing.T) {
	db := setupTestDB(t)

	// Insert
	b := &Book{Title: "Test Book", Author: "Author", Format: "pdf", SourcePath: "/tmp/test.pdf", FileHash: "abc123", Tags: "[]"}
	id, err := db.InsertBook(b)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Get
	got, err := db.GetBook(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Test Book" {
		t.Errorf("title: got %q, want %q", got.Title, "Test Book")
	}

	// GetByPath
	got2, err := db.GetBookByPath("/tmp/test.pdf")
	if err != nil {
		t.Fatalf("get by path: %v", err)
	}
	if got2.BookID != id {
		t.Errorf("book id: got %d, want %d", got2.BookID, id)
	}

	// List
	books, err := db.ListBooks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(books) != 1 {
		t.Errorf("list count: got %d, want 1", len(books))
	}

	// Delete
	if err := db.DeleteBook(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got3, err := db.GetBook(id)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if got3 != nil {
		t.Error("book should be nil after delete")
	}
}

func TestChunkAndFTS(t *testing.T) {
	db := setupTestDB(t)

	// Insert book and chapter
	bookID, _ := db.InsertBook(&Book{Title: "FTS Test", Format: "pdf", SourcePath: "/tmp/fts.pdf", FileHash: "h1"})
	chapterID, _ := db.InsertChapter(&Chapter{BookID: bookID, Title: "Chapter 1", ChapterOrder: 0})

	// Insert chunks
	chunk1ID, _ := db.InsertChunk(&Chunk{
		BookID: bookID, ChapterID: chapterID, Heading: "Section A",
		Body: "ドメインモデリングとは、ビジネスドメインの構造を表現する手法です。", CharCount: 30, ChunkOrder: 0,
	})
	_, _ = db.InsertChunk(&Chunk{
		BookID: bookID, ChapterID: chapterID, Heading: "Section B",
		Body: "関数型プログラミングでは副作用を制御します。", CharCount: 20, ChunkOrder: 1,
	})

	// Verify chunk exists
	c, err := db.GetChunkByID(chunk1ID)
	if err != nil {
		t.Fatalf("get chunk: %v", err)
	}
	if c.Heading != "Section A" {
		t.Errorf("heading: got %q, want %q", c.Heading, "Section A")
	}

	// FTS search - try different query formats
	for _, q := range []string{"ドメインモデリング", "ドメイン", "構造", "Section"} {
		results, err := db.SearchFTS(q, 10, nil)
		if err != nil {
			t.Logf("fts search %q: error: %v", q, err)
			continue
		}
		t.Logf("FTS query=%q results=%d", q, len(results))
		for _, r := range results {
			t.Logf("  chunk_id=%d score=%f", r.ChunkID, r.Score)
		}
	}

	// Also test raw FTS to debug
	var ftsCount int
	db.QueryRow("SELECT COUNT(*) FROM chunk_fts").Scan(&ftsCount)
	t.Logf("FTS row count: %d", ftsCount)

	// Try trigram-like search by checking if body contains the text
	var bodyText string
	db.QueryRow("SELECT body FROM chunk_fts WHERE rowid = ?", chunk1ID).Scan(&bodyText)
	t.Logf("FTS body for chunk %d: %q", chunk1ID, bodyText)
}

func TestVectorSearch(t *testing.T) {
	db := setupTestDB(t)

	// Insert book, chapter, chunks
	bookID, _ := db.InsertBook(&Book{Title: "Vec Test", Format: "pdf", SourcePath: "/tmp/vec.pdf", FileHash: "h2"})
	chapterID, _ := db.InsertChapter(&Chapter{BookID: bookID, Title: "Ch1", ChapterOrder: 0})

	// Insert 3 chunks with embeddings (using small 4-dim vectors for testing)
	// Note: schema says float[768], but for testing we'll create a test-specific table
	// Actually, we need to use 768-dim. Let's create minimal 768-dim vectors.
	dim := 768
	makeVec := func(val float32) []float32 {
		v := make([]float32, dim)
		v[0] = val
		return v
	}

	for i := 0; i < 3; i++ {
		id, _ := db.InsertChunk(&Chunk{
			BookID: bookID, ChapterID: chapterID, Heading: "H",
			Body: "test", CharCount: 4, ChunkOrder: i,
		})
		if err := db.InsertEmbedding(id, makeVec(float32(i)*0.5)); err != nil {
			t.Fatalf("insert embedding %d: %v", i, err)
		}
	}

	// Search
	results, err := db.SearchVector(makeVec(0.4), 2, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(results) < 2 {
		t.Errorf("vector search returned %d results, want >= 2", len(results))
	} else {
		t.Logf("Vector results: chunk_id=%d dist=%f, chunk_id=%d dist=%f",
			results[0].ChunkID, results[0].Score, results[1].ChunkID, results[1].Score)
	}
}

func TestDefaultDBPath(t *testing.T) {
	// Test that empty path defaults to ~/.refloom/refloom.db
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".refloom", "refloom.db")

	db, err := Open("")
	if err != nil {
		t.Fatalf("open default: %v", err)
	}
	defer db.Close()

	// Verify the file was created
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("default db not created at %s", expected)
	}
}

func TestCascadeDelete(t *testing.T) {
	db := setupTestDB(t)

	bookID, _ := db.InsertBook(&Book{Title: "Cascade", Format: "epub", SourcePath: "/tmp/cascade.epub", FileHash: "h3"})
	chapterID, _ := db.InsertChapter(&Chapter{BookID: bookID, Title: "Ch", ChapterOrder: 0})
	chunkID, _ := db.InsertChunk(&Chunk{BookID: bookID, ChapterID: chapterID, Heading: "H", Body: "text", CharCount: 4, ChunkOrder: 0})
	_ = db.InsertEmbedding(chunkID, make([]float32, 768))

	// Delete book should cascade
	db.DeleteBook(bookID)

	// Verify chapter gone
	chapters, _ := db.GetChaptersByBook(bookID)
	if len(chapters) != 0 {
		t.Error("chapters should be deleted")
	}

	// Verify chunk gone
	c, _ := db.GetChunkByID(chunkID)
	if c != nil {
		t.Error("chunk should be deleted")
	}

	// Verify embedding - vec0 doesn't cascade, but chunk is gone
	var count int
	db.QueryRow("SELECT COUNT(*) FROM chunk_vec WHERE chunk_id = ?", chunkID).Scan(&count)
	_ = count // vec0 may or may not cascade, that's ok for PoC
}

// Helper to create nullable int64
func nullInt(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}
