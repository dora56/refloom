-- FTS5 segmented table using unicode61 tokenizer for morphological analysis.
-- Text is pre-segmented (space-separated) by kagome before insertion.
-- Unlike chunk_fts (trigram), this table is populated explicitly by Go code
-- after segmentation, NOT by triggers.

CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts_seg USING fts5(
    heading,
    body,
    tokenize = 'unicode61'
);
