package search

import "strings"

// controlTerms are query fragments that signal comparison intent.
var controlTerms = []string{
	"比較したい",
	"比較",
	"共通点",
	"違い",
	"それぞれ",
	"対比",
	"compare",
	"difference",
	"both",
}

// Intent represents the detected intent of a search query.
type Intent struct {
	IsComparison bool
	ControlTerms []string
}

// DetectIntent analyzes a query string for comparison intent.
func DetectIntent(query string) Intent {
	q := strings.ToLower(query)
	var found []string
	for _, term := range controlTerms {
		if strings.Contains(q, strings.ToLower(term)) {
			found = append(found, term)
		}
	}
	return Intent{
		IsComparison: len(found) > 0,
		ControlTerms: found,
	}
}
