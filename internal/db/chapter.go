package db

import (
	"database/sql"
	"fmt"
)

// Chapter represents a chapter record.
type Chapter struct {
	ChapterID    int64
	BookID       int64
	Title        string
	ChapterOrder int
	PageStart    sql.NullInt64
	PageEnd      sql.NullInt64
}

// InsertChapter inserts a new chapter and returns its ID.
func (db *DB) InsertChapter(c *Chapter) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO chapter (book_id, title, chapter_order, page_start, page_end)
		 VALUES (?, ?, ?, ?, ?)`,
		c.BookID, c.Title, c.ChapterOrder, c.PageStart, c.PageEnd,
	)
	if err != nil {
		return 0, fmt.Errorf("insert chapter: %w", err)
	}
	return res.LastInsertId()
}

// GetChaptersByBook returns all chapters for a book, ordered by chapter_order.
func (db *DB) GetChaptersByBook(bookID int64) ([]*Chapter, error) {
	rows, err := db.Query(
		`SELECT chapter_id, book_id, title, chapter_order, page_start, page_end
		 FROM chapter WHERE book_id = ? ORDER BY chapter_order`, bookID,
	)
	if err != nil {
		return nil, fmt.Errorf("get chapters: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var chapters []*Chapter
	for rows.Next() {
		c := &Chapter{}
		if err := rows.Scan(&c.ChapterID, &c.BookID, &c.Title, &c.ChapterOrder, &c.PageStart, &c.PageEnd); err != nil {
			return nil, fmt.Errorf("scan chapter: %w", err)
		}
		chapters = append(chapters, c)
	}
	return chapters, rows.Err()
}
