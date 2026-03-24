package db

import (
	"strings"

	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
)

var tok *tokenizer.Tokenizer

func init() {
	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		panic("failed to initialize kagome tokenizer: " + err.Error())
	}
	tok = t
}

// SegmentText splits text into space-separated tokens using morphological analysis.
// Suitable for storing in FTS5 with the unicode61 tokenizer.
func SegmentText(text string) string {
	tokens := tok.Tokenize(text)
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		surface := strings.TrimSpace(t.Surface)
		if surface == "" {
			continue
		}
		parts = append(parts, surface)
	}
	return strings.Join(parts, " ")
}

// SegmentQuery splits a query into space-separated tokens for FTS5 MATCH.
// Produces an OR-joined query: "token1 OR token2 OR token3"
func SegmentQuery(query string) string {
	tokens := tok.Tokenize(query)
	var parts []string
	for _, t := range tokens {
		surface := strings.TrimSpace(t.Surface)
		if surface == "" {
			continue
		}
		// Skip single-char particles/punctuation for better precision
		if len([]rune(surface)) == 1 {
			continue
		}
		parts = append(parts, surface)
	}
	if len(parts) == 0 {
		return query // fallback to raw query
	}
	return strings.Join(parts, " OR ")
}
