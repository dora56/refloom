package search

import "testing"

func TestDetectIntent(t *testing.T) {
	tests := []struct {
		query          string
		wantComparison bool
	}{
		{"ドメインモデリング 値オブジェクト", false},
		{"技術文書の書き方とドキュメント運用を比較したい", true},
		{"ドメインモデリングと関数型プログラミングの共通点", true},
		{"境界づけられたコンテキストと型による設計の違い", true},
		{"文章品質とドキュメント運用のそれぞれの特徴", true},
		{"GitLabのドキュメント文化の特徴", false},
		{"compare domain modeling approaches", true},
		{"what is the difference between DDD and FP", true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			intent := DetectIntent(tt.query)
			if intent.IsComparison != tt.wantComparison {
				t.Errorf("DetectIntent(%q).IsComparison = %v, want %v", tt.query, intent.IsComparison, tt.wantComparison)
			}
		})
	}
}
