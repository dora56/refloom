package db

import (
	"database/sql"
	"fmt"
	"sort"
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

// SearchFTS performs a full-text search using both segmented and trigram FTS tables,
// merging results by best score per chunk.
func (db *DB) SearchFTS(query string, limit int, bookID *int64) ([]SearchResult, error) {
	// Segmented search (morphological)
	segQuery := SegmentQuery(query)
	segResults, _ := db.searchFTSTable("chunk_fts_seg", segQuery, limit, bookID)

	// Trigram search
	triResults, _ := db.searchFTSTable("chunk_fts", query, limit, bookID)

	if len(segResults) == 0 && len(triResults) == 0 {
		return nil, fmt.Errorf("no FTS results from either table")
	}

	// Merge: keep best (lowest BM25) score per chunkID
	best := make(map[int64]float64)
	for _, r := range segResults {
		best[r.ChunkID] = r.Score
	}
	for _, r := range triResults {
		if existing, ok := best[r.ChunkID]; !ok || r.Score < existing {
			best[r.ChunkID] = r.Score
		}
	}

	// Sort by score ascending (BM25: lower = better)
	type entry struct {
		id    int64
		score float64
	}
	entries := make([]entry, 0, len(best))
	for id, s := range best {
		entries = append(entries, entry{id, s})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score < entries[j].score
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	results := make([]SearchResult, len(entries))
	for i, e := range entries {
		results[i] = SearchResult{ChunkID: e.id, Score: e.score}
	}
	return results, nil
}

func (db *DB) searchFTSTable(table, query string, limit int, bookID *int64) ([]SearchResult, error) {
	var rows *sql.Rows
	var err error

	if bookID != nil {
		rows, err = db.Query(
			fmt.Sprintf(`SELECT c.chunk_id, bm25(%s) as score
			 FROM %s
			 JOIN chunk c ON c.chunk_id = %s.rowid
			 WHERE %s MATCH ? AND c.book_id = ?
			 ORDER BY score
			 LIMIT ?`, table, table, table, table), query, *bookID, limit,
		)
	} else {
		rows, err = db.Query(
			fmt.Sprintf(`SELECT %s.rowid as chunk_id, bm25(%s) as score
			 FROM %s
			 WHERE %s MATCH ?
			 ORDER BY score
			 LIMIT ?`, table, table, table, table), query, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", table, err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ChunkID, &r.Score); err != nil {
			return nil, fmt.Errorf("scan %s result: %w", table, err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// InsertSegmentedFTS inserts segmented text into chunk_fts_seg for a chunk.
func (db *DB) InsertSegmentedFTS(chunkID int64, heading, body string) error {
	segHeading := SegmentText(heading)
	segBody := SegmentText(body)
	_, err := db.Exec(
		`INSERT INTO chunk_fts_seg(rowid, heading, body) VALUES (?, ?, ?)`,
		chunkID, segHeading, segBody,
	)
	return err
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
