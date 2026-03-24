package db

import (
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// InsertEmbedding inserts a vector embedding for a chunk.
func (db *DB) InsertEmbedding(chunkID int64, embedding []float32) error {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}
	_, err = db.Exec(
		"INSERT INTO chunk_vec(chunk_id, embedding) VALUES (?, ?)",
		chunkID, blob,
	)
	if err != nil {
		return fmt.Errorf("insert embedding: %w", err)
	}
	return nil
}

// DeleteEmbedding removes a vector embedding for a chunk.
func (db *DB) DeleteEmbedding(chunkID int64) error {
	_, err := db.Exec("DELETE FROM chunk_vec WHERE chunk_id = ?", chunkID)
	return err
}

// SearchVector performs a KNN vector search and returns matching chunk IDs with distances.
func (db *DB) SearchVector(queryEmbedding []float32, limit int, bookID *int64) ([]SearchResult, error) {
	blob, err := sqlite_vec.SerializeFloat32(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query: %w", err)
	}

	// sqlite-vec's vec0 MATCH doesn't support JOIN filtering directly.
	// When filtering by book_id, fetch more results and filter in Go.
	fetchLimit := limit
	if bookID != nil {
		fetchLimit = limit * 5
	}

	rows, err := db.Query(
		`SELECT chunk_id, distance
		 FROM chunk_vec
		 WHERE embedding MATCH ? AND k = ?
		 ORDER BY distance`, blob, fetchLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("search vector: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var results []SearchResult
	for rows.Next() {
		var sr SearchResult
		if err := rows.Scan(&sr.ChunkID, &sr.Score); err != nil {
			return nil, fmt.Errorf("scan vector result: %w", err)
		}

		if bookID != nil {
			var chunkBookID int64
			if err := db.QueryRow("SELECT book_id FROM chunk WHERE chunk_id = ?", sr.ChunkID).Scan(&chunkBookID); err != nil {
				continue
			}
			if chunkBookID != *bookID {
				continue
			}
		}

		results = append(results, sr)
		if len(results) >= limit {
			break
		}
	}

	return results, rows.Err()
}
