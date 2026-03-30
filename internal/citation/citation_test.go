package citation

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/search"
)

func makeResult(bookTitle, chapterTitle, body string, pageStart, pageEnd int64) search.Result {
	return search.Result{
		Score:   0.9,
		Book:    &db.Book{Title: bookTitle},
		Chapter: &db.Chapter{Title: chapterTitle},
		Chunk: &db.Chunk{
			Body:      body,
			PageStart: sql.NullInt64{Int64: pageStart, Valid: true},
			PageEnd:   sql.NullInt64{Int64: pageEnd, Valid: true},
		},
	}
}

func TestBuildPromptWithBudget_PerChunkTruncation(t *testing.T) {
	results := []search.Result{
		makeResult("Book A", "Ch1", strings.Repeat("x", 1000), 1, 5),
	}

	_, user := BuildPromptWithBudget("test query", results, PromptOptions{Budget: 0, PerChunk: 100})

	// Body should be truncated to 100 chars + "..."
	if !strings.Contains(user, strings.Repeat("x", 100)+"...") {
		t.Error("chunk body should be truncated to PerChunk limit")
	}
	if strings.Contains(user, strings.Repeat("x", 101)) {
		t.Error("chunk body should not exceed PerChunk limit")
	}
}

func TestBuildPromptWithBudget_BudgetDropsTrailingResults(t *testing.T) {
	results := []search.Result{
		makeResult("Book A", "Ch1", strings.Repeat("a", 200), 1, 5),
		makeResult("Book B", "Ch2", strings.Repeat("b", 200), 6, 10),
		makeResult("Book C", "Ch3", strings.Repeat("c", 200), 11, 15),
	}

	// Very small budget: only first result should fit
	_, user := BuildPromptWithBudget("query", results, PromptOptions{Budget: 300, PerChunk: 0})

	if !strings.Contains(user, "Book A") {
		t.Error("first result should always be included")
	}
	if strings.Contains(user, "Book C") {
		t.Error("trailing results should be dropped when budget exceeded")
	}
}

func TestBuildPromptWithBudget_NoBudget(t *testing.T) {
	results := []search.Result{
		makeResult("Book A", "Ch1", "body1", 1, 5),
		makeResult("Book B", "Ch2", "body2", 6, 10),
	}

	_, user := BuildPromptWithBudget("query", results, PromptOptions{Budget: 0, PerChunk: 0})

	if !strings.Contains(user, "Book A") || !strings.Contains(user, "Book B") {
		t.Error("all results should be included when budget is 0")
	}
}

func TestBuildPrompt_ContainsSystemPrompt(t *testing.T) {
	results := []search.Result{
		makeResult("Book", "Ch", "text", 1, 1),
	}

	system, user := BuildPrompt("question", results)

	if !strings.Contains(system, "reading assistant") {
		t.Error("system prompt should contain role")
	}
	if !strings.Contains(user, "Question: question") {
		t.Error("user prompt should contain the query")
	}
}

func TestBuildPromptWithBudget_EmptyResults(t *testing.T) {
	_, user := BuildPromptWithBudget("query", nil, DefaultPromptOptions())

	if !strings.Contains(user, "Question: query") {
		t.Error("should still contain the query even with no results")
	}
}

func TestFormatSources(t *testing.T) {
	results := []search.Result{
		makeResult("Book A", "Chapter 1", "body", 10, 20),
		{Score: 0.5, Book: &db.Book{Title: "Book B"}, Chunk: &db.Chunk{}},
	}

	out := FormatSources(results)

	if !strings.Contains(out, "[1] Book A, Chapter 1, pp.10-20") {
		t.Errorf("expected full citation, got: %s", out)
	}
	if !strings.Contains(out, "[2] Book B") {
		t.Errorf("expected book-only citation, got: %s", out)
	}
}

func TestBuildPromptWithBudgetExpandContext(t *testing.T) {
	results := []search.Result{
		{
			Score:     0.9,
			PrevChunk: &db.Chunk{Body: "Previous context paragraph."},
			Chunk: &db.Chunk{
				Body:      "Main content paragraph.",
				PageStart: sql.NullInt64{Int64: 10, Valid: true},
				PageEnd:   sql.NullInt64{Int64: 10, Valid: true},
			},
			NextChunk: &db.Chunk{Body: "Next context paragraph."},
			Book:      &db.Book{Title: "Book A"},
			Chapter:   &db.Chapter{Title: "Ch 1"},
		},
	}

	opts := PromptOptions{Budget: 5000, PerChunk: 2000, ExpandContext: true}
	_, user := BuildPromptWithBudget("test?", results, opts)

	if !strings.Contains(user, "Previous context paragraph.") {
		t.Error("expected prev chunk body in expanded context")
	}
	if !strings.Contains(user, "Main content paragraph.") {
		t.Error("expected main chunk body in expanded context")
	}
	if !strings.Contains(user, "Next context paragraph.") {
		t.Error("expected next chunk body in expanded context")
	}
}

func TestBuildPromptWithBudgetExpandContextTruncates(t *testing.T) {
	long := strings.Repeat("A", 500)
	results := []search.Result{
		{
			PrevChunk: &db.Chunk{Body: long},
			Chunk:     &db.Chunk{Body: long, PageStart: sql.NullInt64{Int64: 1, Valid: true}, PageEnd: sql.NullInt64{Int64: 1, Valid: true}},
			NextChunk: &db.Chunk{Body: long},
			Book:      &db.Book{Title: "B"},
			Chapter:   &db.Chapter{Title: "C"},
		},
	}

	// Combined body = 500 + 2(\n\n) + 500 + 2(\n\n) + 500 = 1504 chars. PerChunk=1200 should truncate.
	opts := PromptOptions{Budget: 5000, PerChunk: 1200, ExpandContext: true}
	_, user := BuildPromptWithBudget("q?", results, opts)

	if !strings.Contains(user, "...") {
		t.Error("expected truncation marker")
	}
}

func TestBuildPromptWithBudgetExpandContextNilAdjacentChunks(t *testing.T) {
	results := []search.Result{
		{
			Chunk: &db.Chunk{Body: "Only this.", PageStart: sql.NullInt64{Int64: 1, Valid: true}, PageEnd: sql.NullInt64{Int64: 1, Valid: true}},
			Book:  &db.Book{Title: "B"},
		},
	}

	opts := PromptOptions{Budget: 5000, PerChunk: 2000, ExpandContext: true}
	_, user := BuildPromptWithBudget("q?", results, opts)

	if !strings.Contains(user, "Only this.") {
		t.Error("expected main chunk body even with nil adjacent")
	}
}

func TestFormatSources_NilFields(t *testing.T) {
	results := []search.Result{
		{Score: 0.5},
	}

	out := FormatSources(results)

	if !strings.Contains(out, "[1] Unknown") {
		t.Errorf("nil book should show Unknown, got: %s", out)
	}
}
