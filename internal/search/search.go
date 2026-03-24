package search

import (
	"context"
	"fmt"
	"sort"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
)

// Mode represents the search mode.
type Mode string

const (
	ModeFTS    Mode = "fts"
	ModeVector Mode = "vector"
	ModeHybrid Mode = "hybrid"
)

// Result represents a search result with metadata.
type Result struct {
	ChunkID  int64
	Score    float64
	Chunk    *db.Chunk
	Chapter  *db.Chapter
	Book     *db.Book
}

// Engine performs hybrid search across FTS5 and vector indexes.
type Engine struct {
	DB          *db.DB
	EmbedClient *embedding.Client
}

// NewEngine creates a new search engine.
func NewEngine(database *db.DB, embedClient *embedding.Client) *Engine {
	return &Engine{DB: database, EmbedClient: embedClient}
}

// Search performs a search using the specified mode.
func (e *Engine) Search(ctx context.Context, query string, limit int, mode Mode, bookID *int64) ([]Result, error) {
	fetchK := limit * 3 // fetch more for merging

	switch mode {
	case ModeFTS:
		return e.searchFTS(query, limit, bookID)
	case ModeVector:
		return e.searchVector(ctx, query, limit, bookID)
	case ModeHybrid:
		return e.searchHybrid(ctx, query, fetchK, limit, bookID)
	default:
		return nil, fmt.Errorf("unknown search mode: %s", mode)
	}
}

func (e *Engine) searchFTS(query string, limit int, bookID *int64) ([]Result, error) {
	dbResults, err := e.DB.SearchFTS(query, limit, bookID)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	return e.enrichResults(dbResults)
}

func (e *Engine) searchVector(ctx context.Context, query string, limit int, bookID *int64) ([]Result, error) {
	queryEmb, err := e.EmbedClient.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	dbResults, err := e.DB.SearchVector(queryEmb, limit, bookID)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	return e.enrichResults(dbResults)
}

func (e *Engine) searchHybrid(ctx context.Context, query string, fetchK, limit int, bookID *int64) ([]Result, error) {
	// Run FTS and vector search
	ftsResults, ftsErr := e.DB.SearchFTS(query, fetchK, bookID)
	if ftsErr != nil {
		ftsResults = nil // continue with vector only
	}

	queryEmb, embErr := e.EmbedClient.Embed(ctx, query)
	var vecResults []db.SearchResult
	if embErr == nil {
		vecResults, _ = e.DB.SearchVector(queryEmb, fetchK, bookID)
	}

	if ftsResults == nil && vecResults == nil {
		return nil, fmt.Errorf("both searches failed: fts=%v, embed=%v", ftsErr, embErr)
	}

	// Reciprocal Rank Fusion — fetch more candidates for diversification
	mergeLimit := limit
	intent := DetectIntent(query)
	if intent.IsComparison && bookID == nil {
		mergeLimit = limit * 2 // wider candidate pool for diversification
	}

	merged := reciprocalRankFusion(ftsResults, vecResults, mergeLimit)
	results, err := e.enrichResults(merged)
	if err != nil {
		return nil, err
	}

	// Apply book diversification for comparison queries (only when no book filter)
	if intent.IsComparison && bookID == nil {
		results = DiversifyByBook(results, 2, limit)
	} else if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// reciprocalRankFusion merges two ranked result lists using RRF.
// score = sum(1 / (k + rank)) with k=60
func reciprocalRankFusion(ftsResults, vecResults []db.SearchResult, limit int) []db.SearchResult {
	const k = 60.0
	scores := make(map[int64]float64)

	for rank, r := range ftsResults {
		scores[r.ChunkID] += 1.0 / (k + float64(rank+1))
	}
	for rank, r := range vecResults {
		scores[r.ChunkID] += 1.0 / (k + float64(rank+1))
	}

	// Sort by score descending
	type scored struct {
		chunkID int64
		score   float64
	}
	var sorted_ []scored
	for id, s := range scores {
		sorted_ = append(sorted_, scored{id, s})
	}
	sort.Slice(sorted_, func(i, j int) bool {
		return sorted_[i].score > sorted_[j].score
	})

	if len(sorted_) > limit {
		sorted_ = sorted_[:limit]
	}

	results := make([]db.SearchResult, len(sorted_))
	for i, s := range sorted_ {
		results[i] = db.SearchResult{ChunkID: s.chunkID, Score: s.score}
	}
	return results
}

// enrichResults adds chunk, chapter, and book metadata to search results.
func (e *Engine) enrichResults(dbResults []db.SearchResult) ([]Result, error) {
	results := make([]Result, 0, len(dbResults))
	// Cache books and chapters
	bookCache := make(map[int64]*db.Book)
	chapterCache := make(map[int64]*db.Chapter)

	for _, r := range dbResults {
		chunk, err := e.DB.GetChunkByID(r.ChunkID)
		if err != nil || chunk == nil {
			continue
		}

		book, ok := bookCache[chunk.BookID]
		if !ok {
			book, _ = e.DB.GetBook(chunk.BookID)
			bookCache[chunk.BookID] = book
		}

		chapter, ok := chapterCache[chunk.ChapterID]
		if !ok {
			chapter = findChapter(e.DB, chunk.ChapterID)
			chapterCache[chunk.ChapterID] = chapter
		}

		results = append(results, Result{
			ChunkID: r.ChunkID,
			Score:   r.Score,
			Chunk:   chunk,
			Chapter: chapter,
			Book:    book,
		})
	}
	return results, nil
}

func findChapter(database *db.DB, chapterID int64) *db.Chapter {
	// Simple lookup via query
	var ch db.Chapter
	err := database.QueryRow(
		`SELECT chapter_id, book_id, title, chapter_order, page_start, page_end
		 FROM chapter WHERE chapter_id = ?`, chapterID,
	).Scan(&ch.ChapterID, &ch.BookID, &ch.Title, &ch.ChapterOrder, &ch.PageStart, &ch.PageEnd)
	if err != nil {
		return nil
	}
	return &ch
}
