package search

import (
	"testing"

	"github.com/dora56/refloom/internal/db"
)

func makeResult(bookID int64, score float64) Result {
	return Result{
		Score: score,
		Book:  &db.Book{BookID: bookID, Title: "Book" + string(rune('A'+bookID-1))},
	}
}

func TestDiversifyByBook_TwoBooksPresent(t *testing.T) {
	// 5 results: 3 from book1, 2 from book2 (ordered by score)
	results := []Result{
		makeResult(1, 0.9),
		makeResult(1, 0.8),
		makeResult(1, 0.7),
		makeResult(2, 0.6),
		makeResult(2, 0.5),
	}

	got := DiversifyByBook(results, 2, 5)

	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	// First two should be from different books (round-robin)
	if got[0].Book.BookID == got[1].Book.BookID {
		t.Error("first two results should be from different books")
	}
}

func TestDiversifyByBook_OnlyOneBook(t *testing.T) {
	results := []Result{
		makeResult(1, 0.9),
		makeResult(1, 0.8),
		makeResult(1, 0.7),
	}

	got := DiversifyByBook(results, 2, 3)

	// Only 1 book exists, so just return as-is
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Score != 0.9 {
		t.Error("should preserve original order when only 1 book")
	}
}

func TestDiversifyByBook_LimitTruncates(t *testing.T) {
	results := []Result{
		makeResult(1, 0.9),
		makeResult(2, 0.8),
		makeResult(1, 0.7),
		makeResult(2, 0.6),
	}

	got := DiversifyByBook(results, 2, 2)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Book.BookID == got[1].Book.BookID {
		t.Error("top 2 should be from different books")
	}
}

func TestDiversifyByBook_ThreeBooks(t *testing.T) {
	results := []Result{
		makeResult(1, 0.9),
		makeResult(1, 0.85),
		makeResult(2, 0.8),
		makeResult(3, 0.7),
		makeResult(2, 0.6),
	}

	got := DiversifyByBook(results, 2, 3)

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Round-robin: book1, book2, book3
	books := map[int64]bool{}
	for _, r := range got {
		books[r.Book.BookID] = true
	}
	if len(books) < 2 {
		t.Error("expected at least 2 different books in top 3")
	}
}

func TestDiversifyByBook_Empty(t *testing.T) {
	got := DiversifyByBook(nil, 2, 5)
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}
