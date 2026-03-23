package citation

import (
	"fmt"
	"strings"

	"github.com/dora56/refloom/internal/search"
)

// MaxQuoteLen is the maximum length of a direct quote in characters.
const MaxQuoteLen = 200

// MaxChunkPreview is the maximum chunk text sent to the LLM.
const MaxChunkPreview = 300

// BuildPrompt constructs the LLM prompt from search results and a query.
func BuildPrompt(query string, results []search.Result) (system, user string) {
	system = `You are a reading assistant. The user has a question about books in their personal library.
Based on the excerpts provided below, write a concise summary that answers the question.

Rules:
- Summarize in your own words. Do NOT copy long passages verbatim.
- Any direct quote must be under 100 characters.
- Always cite sources as [Book Title, Chapter, Pages X-Y].
- If the excerpts do not contain enough information, say so honestly.
- Respond in the same language as the user's question.`

	var sb strings.Builder
	sb.WriteString("Excerpts:\n---\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] ", i+1))
		if r.Book != nil {
			sb.WriteString(fmt.Sprintf("Book: %s", r.Book.Title))
		}
		if r.Chapter != nil {
			sb.WriteString(fmt.Sprintf(" | Chapter: %s", r.Chapter.Title))
		}
		if r.Chunk != nil && r.Chunk.PageStart.Valid && r.Chunk.PageEnd.Valid {
			sb.WriteString(fmt.Sprintf(" | Pages: %d-%d", r.Chunk.PageStart.Int64, r.Chunk.PageEnd.Int64))
		}
		sb.WriteString("\n")
		if r.Chunk != nil {
			text := r.Chunk.Body
			if len(text) > MaxChunkPreview {
				text = text[:MaxChunkPreview] + "..."
			}
			sb.WriteString(text)
		}
		sb.WriteString("\n---\n")
	}
	sb.WriteString(fmt.Sprintf("\nQuestion: %s", query))

	return system, sb.String()
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
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, strings.Join(parts, ", ")))
	}
	return sb.String()
}
