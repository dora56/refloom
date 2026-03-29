package db

import (
	"database/sql"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

// InsertEmbedding inserts a vector embedding for a chunk.
func (db *DB) InsertEmbedding(chunkID int64, embedding []float32) error {
	return insertEmbedding(db.DB, chunkID, embedding)
}

// InsertEmbeddingTx inserts a vector embedding within an existing transaction.
func InsertEmbeddingTx(tx *sql.Tx, chunkID int64, embedding []float32) error {
	return insertEmbedding(tx, chunkID, embedding)
}

func insertEmbedding(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, chunkID int64, embedding []float32) error {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}
	_, err = exec.Exec(
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
	return deleteEmbedding(db.DB, chunkID)
}

// DeleteEmbeddingTx deletes a vector embedding within an existing transaction.
func DeleteEmbeddingTx(tx *sql.Tx, chunkID int64) error {
	return deleteEmbedding(tx, chunkID)
}

func deleteEmbedding(exec interface {
	Exec(query string, args ...any) (sql.Result, error)
}, chunkID int64) error {
	_, err := exec.Exec("DELETE FROM chunk_vec WHERE chunk_id = ?", chunkID)
	return err
}

// SaveEmbeddingBatchTx inserts or replaces a batch of embeddings within an existing transaction.
func SaveEmbeddingBatchTx(tx *sql.Tx, model string, chunkIDs []int64, embeddings [][]float32, replaceExisting bool) error {
	if len(chunkIDs) != len(embeddings) {
		return fmt.Errorf("chunk id / embedding count mismatch: %d != %d", len(chunkIDs), len(embeddings))
	}

	var (
		deleteStmt *sql.Stmt
		err        error
	)
	if replaceExisting {
		deleteStmt, err = tx.Prepare("DELETE FROM chunk_vec WHERE chunk_id = ?")
		if err != nil {
			return fmt.Errorf("prepare delete embedding: %w", err)
		}
		defer deleteStmt.Close() //nolint:errcheck
	}

	insertStmt, err := tx.Prepare("INSERT INTO chunk_vec(chunk_id, embedding) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert embedding: %w", err)
	}
	defer insertStmt.Close() //nolint:errcheck

	updateStmt, err := tx.Prepare("UPDATE chunk SET embedding_version = ? WHERE chunk_id = ?")
	if err != nil {
		return fmt.Errorf("prepare update embedding version: %w", err)
	}
	defer updateStmt.Close() //nolint:errcheck

	for i, chunkID := range chunkIDs {
		if replaceExisting {
			if _, err := deleteStmt.Exec(chunkID); err != nil {
				return fmt.Errorf("delete embedding: %w", err)
			}
		}

		blob, err := sqlite_vec.SerializeFloat32(embeddings[i])
		if err != nil {
			return fmt.Errorf("serialize embedding: %w", err)
		}

		if _, err := insertStmt.Exec(chunkID, blob); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
		if _, err := updateStmt.Exec(model, chunkID); err != nil {
			return fmt.Errorf("update embedding version: %w", err)
		}
	}

	return nil
}

// BuildBinaryIndex creates binary quantized vectors from existing float32 embeddings.
func (db *DB) BuildBinaryIndex() (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Clear existing binary index
	if _, err := tx.Exec("DELETE FROM chunk_vec_binary"); err != nil {
		return 0, fmt.Errorf("clear binary index: %w", err)
	}

	// Quantize float32 → binary using sqlite-vec's built-in function
	result, err := tx.Exec(`
		INSERT INTO chunk_vec_binary(chunk_id, embedding)
		SELECT chunk_id, vec_quantize_binary(embedding)
		FROM chunk_vec
	`)
	if err != nil {
		return 0, fmt.Errorf("build binary index: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit binary index: %w", err)
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

// SearchVectorBinary performs a 2-phase search:
// Phase 1: Binary hamming distance for fast candidate selection (limit * oversampling)
// Phase 2: Float32 cosine re-rank of candidates for precision
func (db *DB) SearchVectorBinary(queryEmbedding []float32, limit int, bookID *int64) ([]SearchResult, error) {
	const oversampleFactor = 5

	queryBlob, err := sqlite_vec.SerializeFloat32(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query: %w", err)
	}

	candidateLimit := limit * oversampleFactor
	if bookID != nil {
		candidateLimit = candidateLimit * 5 // extra oversampling for book filter
	}

	// Phase 1: Binary hamming distance search
	rows, err := db.Query(`
		SELECT chunk_id, distance
		FROM chunk_vec_binary
		WHERE embedding MATCH vec_quantize_binary(?) AND k = ?
		ORDER BY distance
	`, queryBlob, candidateLimit)
	if err != nil {
		return nil, fmt.Errorf("binary search: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var candidateIDs []int64
	for rows.Next() {
		var chunkID int64
		var dist float64
		if err := rows.Scan(&chunkID, &dist); err != nil {
			return nil, fmt.Errorf("scan binary result: %w", err)
		}

		if bookID != nil {
			var chunkBookID int64
			if err := db.QueryRow("SELECT book_id FROM chunk WHERE chunk_id = ?", chunkID).Scan(&chunkBookID); err != nil {
				continue
			}
			if chunkBookID != *bookID {
				continue
			}
		}

		candidateIDs = append(candidateIDs, chunkID)
		if bookID == nil && len(candidateIDs) >= limit*oversampleFactor {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("binary search rows: %w", err)
	}

	if len(candidateIDs) == 0 {
		return nil, nil
	}

	// Phase 2: Re-rank candidates using float32 cosine distance
	var results []SearchResult
	for _, chunkID := range candidateIDs {
		var dist float64
		err := db.QueryRow(`
			SELECT vec_distance_cosine(embedding, ?)
			FROM chunk_vec
			WHERE chunk_id = ?
		`, queryBlob, chunkID).Scan(&dist)
		if err != nil {
			continue // chunk may not have float32 embedding
		}
		results = append(results, SearchResult{ChunkID: chunkID, Score: dist})
	}

	// Sort by cosine distance ascending (lower = more similar)
	sortSearchResults(results)

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func sortSearchResults(results []SearchResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score < results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
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
