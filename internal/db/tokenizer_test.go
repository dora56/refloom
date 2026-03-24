package db

import (
	"strings"
	"testing"
)

func TestSegmentText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // expected tokens (subset)
	}{
		{
			name:  "japanese sentence",
			input: "ドメインモデリングとは何ですか",
			want:  []string{"ドメイン", "モデリング"},
		},
		{
			name:  "mixed",
			input: "技術文書の書き方",
			want:  []string{"技術", "文書", "書き方"},
		},
		{
			name:  "english",
			input: "hello world",
			want:  []string{"hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SegmentText(tt.input)
			for _, w := range tt.want {
				if !strings.Contains(result, w) {
					t.Errorf("SegmentText(%q) = %q, missing token %q", tt.input, result, w)
				}
			}
			if result == "" {
				t.Errorf("SegmentText(%q) returned empty", tt.input)
			}
		})
	}
}

func TestSegmentQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // expected tokens in OR query
	}{
		{
			name:  "japanese query",
			input: "文章",
			want:  []string{"文章"},
		},
		{
			name:  "multi-word query",
			input: "技術文書の書き方",
			want:  []string{"技術", "文書", "書き方"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SegmentQuery(tt.input)
			for _, w := range tt.want {
				if !strings.Contains(result, w) {
					t.Errorf("SegmentQuery(%q) = %q, missing token %q", tt.input, result, w)
				}
			}
			if !strings.Contains(result, " OR ") && len(tt.want) > 1 {
				t.Errorf("SegmentQuery(%q) = %q, expected OR-joined query", tt.input, result)
			}
		})
	}
}

func TestSegmentText_Empty(t *testing.T) {
	result := SegmentText("")
	if result != "" {
		t.Errorf("SegmentText(\"\") = %q, want empty", result)
	}
}
