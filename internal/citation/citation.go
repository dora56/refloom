package citation

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dora56/refloom/internal/search"
)

// PromptOptions controls prompt budget enforcement.
type PromptOptions struct {
	Budget        int  // max total chars for the excerpt section (0 = no limit)
	PerChunk      int  // max chars per chunk body (0 = no limit)
	ExpandContext bool // include adjacent chunks in context
}

// DefaultPromptOptions returns default budget values.
func DefaultPromptOptions() PromptOptions {
	return PromptOptions{Budget: 3000, PerChunk: 500}
}

const systemPrompt = `You are a reading assistant. The user has a question about books in their personal library.
Based on the excerpts provided below, write a concise summary that answers the question.

Rules:
- Summarize in your own words. Do NOT copy long passages verbatim.
- Any direct quote must be under 100 characters.
- Always cite sources as [Book Title, Chapter, Pages X-Y].
- If the excerpts do not contain enough information, say so honestly.
- Respond in the same language as the user's question.`

// BuildPrompt constructs the LLM prompt with default budget.
func BuildPrompt(query string, results []search.Result) (system, user string) {
	return BuildPromptWithBudget(query, results, DefaultPromptOptions())
}

// BuildPromptWithBudget constructs the LLM prompt with explicit budget control.
// Chunks are capped at opts.PerChunk chars each. If the total excerpt section
// would exceed opts.Budget, trailing results are dropped.
func BuildPromptWithBudget(query string, results []search.Result, opts PromptOptions) (system, user string) {
	var sb strings.Builder
	sb.WriteString("Excerpts:\n---\n")
	totalLen := 0

	for i, r := range results {
		// Build this entry
		var entry strings.Builder
		fmt.Fprintf(&entry, "[%d] ", i+1)
		if r.Book != nil {
			fmt.Fprintf(&entry, "Book: %s", r.Book.Title)
		}
		if r.Chapter != nil {
			fmt.Fprintf(&entry, " | Chapter: %s", r.Chapter.Title)
		}
		if r.Chunk != nil && r.Chunk.PageStart.Valid && r.Chunk.PageEnd.Valid {
			fmt.Fprintf(&entry, " | Pages: %d-%d", r.Chunk.PageStart.Int64, r.Chunk.PageEnd.Int64)
		}
		entry.WriteString("\n")
		if r.Chunk != nil {
			var text string
			if opts.ExpandContext {
				var parts []string
				if r.PrevChunk != nil {
					parts = append(parts, r.PrevChunk.Body)
				}
				parts = append(parts, r.Chunk.Body)
				if r.NextChunk != nil {
					parts = append(parts, r.NextChunk.Body)
				}
				text = strings.Join(parts, "\n\n")
			} else {
				text = r.Chunk.Body
			}
			if opts.PerChunk > 0 && len(text) > opts.PerChunk {
				text = truncateUTF8(text, opts.PerChunk) + "..."
			}
			entry.WriteString(text)
		}
		entry.WriteString("\n---\n")

		entryStr := entry.String()

		// Check budget
		if opts.Budget > 0 && totalLen+len(entryStr) > opts.Budget && i > 0 {
			break // drop remaining results
		}

		sb.WriteString(entryStr)
		totalLen += len(entryStr)
	}

	fmt.Fprintf(&sb, "\nQuestion: %s", query)

	return systemPrompt, sb.String()
}

// truncateUTF8 truncates text to at most maxBytes without splitting a multi-byte rune.
func truncateUTF8(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	// Walk back from maxBytes to find a valid rune boundary
	for maxBytes > 0 && !utf8.RuneStart(text[maxBytes]) {
		maxBytes--
	}
	return text[:maxBytes]
}

// FormatSources formats citation sources for display.
func FormatSources(results []search.Result) string {
	var sb strings.Builder
	sb.WriteString("Sources:\n")
	for i, r := range results {
		title := "Unknown"
		chapter := ""
		pages := ""
		if r.Book != nil {
			title = r.Book.Title
		}
		if r.Chapter != nil {
			chapter = r.Chapter.Title
		}
		if r.Chunk != nil && r.Chunk.PageStart.Valid && r.Chunk.PageEnd.Valid {
			pages = fmt.Sprintf("pp.%d-%d", r.Chunk.PageStart.Int64, r.Chunk.PageEnd.Int64)
		}
		parts := []string{title}
		if chapter != "" {
			parts = append(parts, chapter)
		}
		if pages != "" {
			parts = append(parts, pages)
		}
		fmt.Fprintf(&sb, "[%d] %s\n", i+1, strings.Join(parts, ", "))
	}
	return sb.String()
}
