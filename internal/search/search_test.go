package search

import (
	"testing"

	"github.com/dora56/refloom/internal/db"
)

func TestReciprocalRankFusionMergesBothSources(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{
		{ChunkID: 1, Score: 10},
		{ChunkID: 2, Score: 5},
	}
	vec := []db.SearchResult{
		{ChunkID: 2, Score: 0.9},
		{ChunkID: 3, Score: 0.8},
	}

	merged := reciprocalRankFusion(fts, vec, 10)

	// ChunkID 2 appears in both → should have highest RRF score
	if len(merged) != 3 {
		t.Fatalf("len = %d, want 3", len(merged))
	}
	if merged[0].ChunkID != 2 {
		t.Fatalf("top result ChunkID = %d, want 2 (appears in both)", merged[0].ChunkID)
	}
}

func TestReciprocalRankFusionLimit(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{
		{ChunkID: 1}, {ChunkID: 2}, {ChunkID: 3},
	}
	vec := []db.SearchResult{
		{ChunkID: 4}, {ChunkID: 5},
	}

	merged := reciprocalRankFusion(fts, vec, 3)
	if len(merged) != 3 {
		t.Fatalf("len = %d, want 3 (limited)", len(merged))
	}
}

func TestReciprocalRankFusionFTSOnly(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{{ChunkID: 10}, {ChunkID: 20}}
	merged := reciprocalRankFusion(fts, nil, 10)

	if len(merged) != 2 {
		t.Fatalf("len = %d, want 2", len(merged))
	}
	if merged[0].ChunkID != 10 {
		t.Fatalf("first ChunkID = %d, want 10", merged[0].ChunkID)
	}
}

func TestReciprocalRankFusionVectorOnly(t *testing.T) {
	t.Parallel()

	vec := []db.SearchResult{{ChunkID: 30}, {ChunkID: 40}}
	merged := reciprocalRankFusion(nil, vec, 10)

	if len(merged) != 2 {
		t.Fatalf("len = %d, want 2", len(merged))
	}
}

func TestReciprocalRankFusionBothEmpty(t *testing.T) {
	t.Parallel()

	merged := reciprocalRankFusion(nil, nil, 10)
	if len(merged) != 0 {
		t.Fatalf("len = %d, want 0", len(merged))
	}
}

func TestReciprocalRankFusionScoreIsPositive(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{{ChunkID: 1}}
	vec := []db.SearchResult{{ChunkID: 1}}
	merged := reciprocalRankFusion(fts, vec, 10)

	if len(merged) != 1 {
		t.Fatalf("len = %d, want 1", len(merged))
	}
	if merged[0].Score <= 0 {
		t.Fatalf("Score = %f, want > 0", merged[0].Score)
	}
	// Same item in both lists at rank 1: score = 2 * 1/(60+1) ≈ 0.0328
	expected := 2.0 / 61.0
	if merged[0].Score < expected*0.99 || merged[0].Score > expected*1.01 {
		t.Fatalf("Score = %f, want ≈ %f", merged[0].Score, expected)
	}
}
