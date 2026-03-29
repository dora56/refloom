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

	merged := reciprocalRankFusion(10, fts, vec)

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

	merged := reciprocalRankFusion(3, fts, vec)
	if len(merged) != 3 {
		t.Fatalf("len = %d, want 3 (limited)", len(merged))
	}
}

func TestReciprocalRankFusionFTSOnly(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{{ChunkID: 10}, {ChunkID: 20}}
	merged := reciprocalRankFusion(10, fts)

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
	merged := reciprocalRankFusion(10, vec)

	if len(merged) != 2 {
		t.Fatalf("len = %d, want 2", len(merged))
	}
}

func TestReciprocalRankFusionBothEmpty(t *testing.T) {
	t.Parallel()

	merged := reciprocalRankFusion(10)
	if len(merged) != 0 {
		t.Fatalf("len = %d, want 0", len(merged))
	}
}

func TestReciprocalRankFusionThreeWayMerge(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{{ChunkID: 1}, {ChunkID: 2}}
	vec := []db.SearchResult{{ChunkID: 2}, {ChunkID: 3}}
	hyde := []db.SearchResult{{ChunkID: 3}, {ChunkID: 4}}

	merged := reciprocalRankFusion(10, fts, vec, hyde)

	if len(merged) != 4 {
		t.Fatalf("len = %d, want 4", len(merged))
	}
	// ChunkID 2 and 3 each appear in two lists → should rank higher
	top2 := map[int64]bool{merged[0].ChunkID: true, merged[1].ChunkID: true}
	if !top2[2] || !top2[3] {
		t.Fatalf("top 2 = {%d, %d}, want {2, 3} (each in 2 lists)", merged[0].ChunkID, merged[1].ChunkID)
	}
}

func TestReciprocalRankFusionAllThreeLists(t *testing.T) {
	t.Parallel()

	// Item appearing in all 3 lists should rank highest
	fts := []db.SearchResult{{ChunkID: 10}, {ChunkID: 20}}
	vec := []db.SearchResult{{ChunkID: 10}, {ChunkID: 30}}
	hyde := []db.SearchResult{{ChunkID: 10}, {ChunkID: 40}}

	merged := reciprocalRankFusion(10, fts, vec, hyde)
	if merged[0].ChunkID != 10 {
		t.Fatalf("top ChunkID = %d, want 10 (in all 3 lists)", merged[0].ChunkID)
	}
	// Score should be 3 * 1/(60+1)
	expected := 3.0 / 61.0
	if merged[0].Score < expected*0.99 || merged[0].Score > expected*1.01 {
		t.Fatalf("Score = %f, want ≈ %f", merged[0].Score, expected)
	}
}

func TestReciprocalRankFusionSingleList(t *testing.T) {
	t.Parallel()

	hyde := []db.SearchResult{{ChunkID: 50}, {ChunkID: 60}}
	merged := reciprocalRankFusion(10, hyde)
	if len(merged) != 2 {
		t.Fatalf("len = %d, want 2", len(merged))
	}
}

func TestReciprocalRankFusionScoreIsPositive(t *testing.T) {
	t.Parallel()

	fts := []db.SearchResult{{ChunkID: 1}}
	vec := []db.SearchResult{{ChunkID: 1}}
	merged := reciprocalRankFusion(10, fts, vec)

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
