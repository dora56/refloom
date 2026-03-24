package search

import (
	"strings"
	"testing"
)

func TestExpandQuery_WithAlias(t *testing.T) {
	result := ExpandQuery("文章品質とドキュメント運用の共通点")
	// Should contain original plus alias terms
	if !strings.Contains(result, "文章品質") {
		t.Error("should contain original query")
	}
	if !strings.Contains(result, "読みやすさ") {
		t.Error("should contain alias term '読みやすさ'")
	}
	if !strings.Contains(result, "ハンドブック") {
		t.Error("should contain alias term 'ハンドブック'")
	}
}

func TestExpandQuery_NoAlias(t *testing.T) {
	query := "SQLiteの使い方"
	result := ExpandQuery(query)
	if result != query {
		t.Errorf("got %q, want %q (no aliases should match)", result, query)
	}
}

func TestExpandQuery_NoDuplicates(t *testing.T) {
	result := ExpandQuery("技術文書の書き方")
	// "技術" and "文書" are in the query already, should not be duplicated
	count := strings.Count(result, "技術")
	if count > 1 {
		t.Errorf("'技術' appears %d times, expected at most 1", count)
	}
}
