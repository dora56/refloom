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
	ChunkID   int64
	Score     float64
	PrevChunk *db.Chunk // adjacent chunk (same chapter only)
	Chunk     *db.Chunk
	NextChunk *db.Chunk // adjacent chunk (same chapter only)
	Chapter   *db.Chapter
	Book      *db.Book
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
	expanded := ExpandQuery(query)
	dbResults, err := e.DB.SearchFTS(expanded, limit, bookID)
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

// SearchHybridWithHyDE performs hybrid search augmented with a HyDE hypothesis.
// It runs FTS + original vector + hypothesis vector, then merges all three via RRF.
func (e *Engine) SearchHybridWithHyDE(ctx context.Context, query, hypothesis string, limit int, bookID *int64) ([]Result, error) {
	fetchK := limit * 3

	// FTS with original query
	expanded := ExpandQuery(query)
	ftsResults, ftsErr := e.DB.SearchFTS(expanded, fetchK, bookID)
	if ftsErr != nil {
		ftsResults = nil
	}

	// Vector search with original query
	var vecErr error
	queryEmb, embErr := e.EmbedClient.Embed(ctx, query)
	var vecResults []db.SearchResult
	if embErr == nil {
		vecResults, vecErr = e.DB.SearchVector(queryEmb, fetchK, bookID)
		if vecErr != nil {
			vecResults = nil
		}
	}

	// Vector search with hypothesis
	var hydeVecErr error
	hydeEmb, hydeErr := e.EmbedClient.Embed(ctx, hypothesis)
	var hydeResults []db.SearchResult
	if hydeErr == nil {
		hydeResults, hydeVecErr = e.DB.SearchVector(hydeEmb, fetchK, bookID)
		if hydeVecErr != nil {
			hydeResults = nil
		}
	}

	if ftsResults == nil && vecResults == nil && hydeResults == nil {
		return nil, fmt.Errorf("all searches failed: fts=%v, embed=%v, vec=%v, hyde_embed=%v, hyde_vec=%v",
			ftsErr, embErr, vecErr, hydeErr, hydeVecErr)
	}

	return e.mergeAndDiversify(query, limit, bookID, ftsResults, vecResults, hydeResults)
}

func (e *Engine) searchHybrid(ctx context.Context, query string, fetchK, limit int, bookID *int64) ([]Result, error) {
	// Run FTS and vector search
	expanded := ExpandQuery(query)
	ftsResults, ftsErr := e.DB.SearchFTS(expanded, fetchK, bookID)
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

	return e.mergeAndDiversify(query, limit, bookID, ftsResults, vecResults)
}

// mergeAndDiversify applies RRF merge and book diversification to ranked result lists.
func (e *Engine) mergeAndDiversify(query string, limit int, bookID *int64, lists ...[]db.SearchResult) ([]Result, error) {
	mergeLimit := limit
	intent := DetectIntent(query)
	if intent.IsComparison && bookID == nil {
		mergeLimit = limit * 2 // wider candidate pool for diversification
	}

	merged := reciprocalRankFusion(mergeLimit, lists...)
	results, err := e.enrichResults(merged)
	if err != nil {
		return nil, err
	}

	if intent.IsComparison && bookID == nil {
		results = DiversifyByBook(results, 2, limit)
	} else if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// reciprocalRankFusion merges ranked result lists using RRF.
// score = sum(1 / (k + rank)) with k=60
func reciprocalRankFusion(limit int, lists ...[]db.SearchResult) []db.SearchResult {
	const k = 60.0
	scores := make(map[int64]float64)

	for _, list := range lists {
		for rank, r := range list {
			scores[r.ChunkID] += 1.0 / (k + float64(rank+1))
		}
	}

	type scored struct {
		chunkID int64
		score   float64
	}
	var ranked []scored
	for id, s := range scores {
		ranked = append(ranked, scored{id, s})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	results := make([]db.SearchResult, len(ranked))
	for i, s := range ranked {
		results[i] = db.SearchResult{ChunkID: s.chunkID, Score: s.score}
	}
	return results
}

// EnrichWithAdjacentChunks populates PrevChunk/NextChunk for results that already have metadata.
func (e *Engine) EnrichWithAdjacentChunks(results []Result) {
	seen := make(map[int64]bool)
	for _, r := range results {
		if r.Chunk != nil {
			seen[r.Chunk.ChunkID] = true
		}
	}

	for i := range results {
		chunk := results[i].Chunk
		if chunk == nil {
			continue
		}

		if chunk.PrevChunkID.Valid && !seen[chunk.PrevChunkID.Int64] {
			if pc, err := e.DB.GetChunkByID(chunk.PrevChunkID.Int64); err == nil && pc != nil && pc.ChapterID == chunk.ChapterID {
				results[i].PrevChunk = pc
				seen[pc.ChunkID] = true
			}
		}
		if chunk.NextChunkID.Valid && !seen[chunk.NextChunkID.Int64] {
			if nc, err := e.DB.GetChunkByID(chunk.NextChunkID.Int64); err == nil && nc != nil && nc.ChapterID == chunk.ChapterID {
				results[i].NextChunk = nc
				seen[nc.ChunkID] = true
			}
		}
	}
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
