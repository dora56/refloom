package db

import "fmt"

// LogIngest records an ingest status entry.
func (db *DB) LogIngest(bookID int64, status, message string) error {
	_, err := db.Exec(
		`INSERT INTO ingest_log (book_id, status, message) VALUES (?, ?, ?)`,
		bookID, status, message,
	)
	if err != nil {
		return fmt.Errorf("log ingest: %w", err)
	}
	return nil
}

// CountChunksWithoutEmbedding returns the number of chunks that have no vector embedding.
func (db *DB) CountChunksWithoutEmbedding() (int, error) {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM chunk c
		 WHERE NOT EXISTS (SELECT 1 FROM chunk_vec cv WHERE cv.chunk_id = c.chunk_id)`,
	).Scan(&count)
	return count, err
}
