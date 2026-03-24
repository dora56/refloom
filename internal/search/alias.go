package search

import "strings"

// aliases maps terms to their synonyms for query expansion.
var aliases = map[string][]string{
	"文章品質":       {"文書", "品質", "読みやすさ", "テクニカルライティング"},
	"ドキュメント運用":  {"ドキュメント", "ハンドブック", "運用"},
	"技術文書":       {"技術", "文書", "テクニカルライティング"},
	"境界設計":       {"境界", "コンテキスト", "境界づけられた"},
	"テナント境界":     {"テナント", "境界", "分離"},
	"ドメインモデリング":  {"ドメイン", "モデリング", "DDD"},
	"関数型プログラミング": {"関数型", "型", "モデリング"},
}

// ExpandQuery appends alias terms to the query for broader FTS recall.
// Returns the original query with additional terms appended.
func ExpandQuery(query string) string {
	q := strings.ToLower(query)
	var extra []string
	for key, synonyms := range aliases {
		if strings.Contains(q, strings.ToLower(key)) {
			extra = append(extra, synonyms...)
		}
	}
	if len(extra) == 0 {
		return query
	}
	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, s := range extra {
		low := strings.ToLower(s)
		if !seen[low] && !strings.Contains(q, low) {
			seen[low] = true
			unique = append(unique, s)
		}
	}
	if len(unique) == 0 {
		return query
	}
	return query + " " + strings.Join(unique, " ")
}
