package db

import (
	"database/sql"
	"fmt"
)

// Book represents a book record.
type Book struct {
	BookID     int64
	Title      string
	Author     string
	Format     string
	SourcePath string
	FileHash   string
	Tags       string
	IngestedAt string
	UpdatedAt  string
}

// InsertBook inserts a new book and returns its ID.
func (db *DB) InsertBook(b *Book) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO book (title, author, format, source_path, file_hash, tags)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.Title, b.Author, b.Format, b.SourcePath, b.FileHash, b.Tags,
	)
	if err != nil {
		return 0, fmt.Errorf("insert book: %w", err)
	}
	return res.LastInsertId()
}

// GetBook retrieves a book by ID.
func (db *DB) GetBook(bookID int64) (*Book, error) {
	b := &Book{}
	err := db.QueryRow(
		`SELECT book_id, title, author, format, source_path, file_hash, tags, ingested_at, updated_at
		 FROM book WHERE book_id = ?`, bookID,
	).Scan(&b.BookID, &b.Title, &b.Author, &b.Format, &b.SourcePath, &b.FileHash, &b.Tags, &b.IngestedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book: %w", err)
	}
	return b, nil
}

// GetBookByPath retrieves a book by source path.
func (db *DB) GetBookByPath(sourcePath string) (*Book, error) {
	b := &Book{}
	err := db.QueryRow(
		`SELECT book_id, title, author, format, source_path, file_hash, tags, ingested_at, updated_at
		 FROM book WHERE source_path = ?`, sourcePath,
	).Scan(&b.BookID, &b.Title, &b.Author, &b.Format, &b.SourcePath, &b.FileHash, &b.Tags, &b.IngestedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book by path: %w", err)
	}
	return b, nil
}

// GetBookByHash retrieves a book by file hash.
func (db *DB) GetBookByHash(fileHash string) (*Book, error) {
	b := &Book{}
	err := db.QueryRow(
		`SELECT book_id, title, author, format, source_path, file_hash, tags, ingested_at, updated_at
		 FROM book WHERE file_hash = ?`, fileHash,
	).Scan(&b.BookID, &b.Title, &b.Author, &b.Format, &b.SourcePath, &b.FileHash, &b.Tags, &b.IngestedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book by hash: %w", err)
	}
	return b, nil
}

// ListBooks returns all books.
func (db *DB) ListBooks() ([]*Book, error) {
	rows, err := db.Query(
		`SELECT book_id, title, author, format, source_path, file_hash, tags, ingested_at, updated_at
		 FROM book ORDER BY book_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var books []*Book
	for rows.Next() {
		b := &Book{}
		if err := rows.Scan(&b.BookID, &b.Title, &b.Author, &b.Format, &b.SourcePath, &b.FileHash, &b.Tags, &b.IngestedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan book: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// DeleteBook deletes a book and all related data (cascades).
func (db *DB) DeleteBook(bookID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete book tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`UPDATE chunk
		 SET prev_chunk_id = NULL, next_chunk_id = NULL
		 WHERE book_id = ?`,
		bookID,
	); err != nil {
		return fmt.Errorf("clear chunk links: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM chunk_vec
		 WHERE chunk_id IN (SELECT chunk_id FROM chunk WHERE book_id = ?)`,
		bookID,
	); err != nil {
		return fmt.Errorf("delete embeddings: %w", err)
	}

	if _, err := tx.Exec(
		`DELETE FROM chunk_fts_seg
		 WHERE rowid IN (SELECT chunk_id FROM chunk WHERE book_id = ?)`,
		bookID,
	); err != nil {
		return fmt.Errorf("delete segmented fts rows: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM ingest_log WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("delete ingest log: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM book WHERE book_id = ?", bookID); err != nil {
		return fmt.Errorf("delete book: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete book tx: %w", err)
	}
	return nil
}
