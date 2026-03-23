package db

import (
	"database/sql"
	"fmt"
)

// Chunk represents a chunk record.
type Chunk struct {
	ChunkID          int64
	BookID           int64
	ChapterID        int64
	Heading          string
	Body             string
	CharCount        int
	PageStart        sql.NullInt64
	PageEnd          sql.NullInt64
	ChunkOrder       int
	PrevChunkID      sql.NullInt64
	NextChunkID      sql.NullInt64
	EmbeddingVersion string
	CreatedAt        string
}

// InsertChunk inserts a new chunk and returns its ID.
// FTS5 sync is handled by the AFTER INSERT trigger.
func (db *DB) InsertChunk(c *Chunk) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO chunk (book_id, chapter_id, heading, body, char_count, page_start, page_end,
		                     chunk_order, prev_chunk_id, next_chunk_id, embedding_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.BookID, c.ChapterID, c.Heading, c.Body, c.CharCount,
		c.PageStart, c.PageEnd, c.ChunkOrder, c.PrevChunkID, c.NextChunkID, c.EmbeddingVersion,
	)
	if err != nil {
		return 0, fmt.Errorf("insert chunk: %w", err)
	}
	return res.LastInsertId()
}

// GetChunkByID retrieves a single chunk by ID.
func (db *DB) GetChunkByID(chunkID int64) (*Chunk, error) {
	c := &Chunk{}
	err := db.QueryRow(
		`SELECT chunk_id, book_id, chapter_id, heading, body, char_count, page_start, page_end,
		        chunk_order, prev_chunk_id, next_chunk_id, embedding_version, created_at
		 FROM chunk WHERE chunk_id = ?`, chunkID,
	).Scan(&c.ChunkID, &c.BookID, &c.ChapterID, &c.Heading, &c.Body, &c.CharCount,
		&c.PageStart, &c.PageEnd, &c.ChunkOrder, &c.PrevChunkID, &c.NextChunkID, &c.EmbeddingVersion, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chunk: %w", err)
	}
	return c, nil
}

// GetChunksByBook returns all chunks for a book.
func (db *DB) GetChunksByBook(bookID int64) ([]*Chunk, error) {
	rows, err := db.Query(
		`SELECT chunk_id, book_id, chapter_id, heading, body, char_count, page_start, page_end,
		        chunk_order, prev_chunk_id, next_chunk_id, embedding_version, created_at
		 FROM chunk WHERE book_id = ? ORDER BY chapter_id, chunk_order`, bookID,
	)
	if err != nil {
		return nil, fmt.Errorf("get chunks by book: %w", err)
	}
	defer rows.Close()

	return scanChunks(rows)
}

// CountChunksByBook returns the number of chunks for a book.
func (db *DB) CountChunksByBook(bookID int64) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM chunk WHERE book_id = ?", bookID).Scan(&count)
	return count, err
}

// SearchFTS performs a full-text search and returns matching chunk IDs with BM25 scores.
func (db *DB) SearchFTS(query string, limit int, bookID *int64) ([]SearchResult, error) {
	var rows *sql.Rows
	var err error

	if bookID != nil {
		rows, err = db.Query(
			`SELECT c.chunk_id, bm25(chunk_fts) as score
			 FROM chunk_fts
			 JOIN chunk c ON c.chunk_id = chunk_fts.rowid
			 WHERE chunk_fts MATCH ? AND c.book_id = ?
			 ORDER BY score
			 LIMIT ?`, query, *bookID, limit,
		)
	} else {
		rows, err = db.Query(
			`SELECT chunk_fts.rowid as chunk_id, bm25(chunk_fts) as score
			 FROM chunk_fts
			 WHERE chunk_fts MATCH ?
			 ORDER BY score
			 LIMIT ?`, query, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("search fts: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ChunkID, &r.Score); err != nil {
			return nil, fmt.Errorf("scan fts result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// SearchResult holds a search result with chunk ID and score.
type SearchResult struct {
	ChunkID int64
	Score   float64
}

func scanChunks(rows *sql.Rows) ([]*Chunk, error) {
	var chunks []*Chunk
	for rows.Next() {
		c := &Chunk{}
		if err := rows.Scan(&c.ChunkID, &c.BookID, &c.ChapterID, &c.Heading, &c.Body, &c.CharCount,
			&c.PageStart, &c.PageEnd, &c.ChunkOrder, &c.PrevChunkID, &c.NextChunkID, &c.EmbeddingVersion, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chunk: %w", err)
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}
