"""Tests for EPUB text cleaning logic."""

from refloom_worker.epub_extractor import clean_text


class TestCleanText:
    """Tests for clean_text()."""

    def test_join_single_char_lines(self):
        """Consecutive single-char lines (layout-split titles) are joined."""
        assert clean_text("第\n一\n章") == "第一章"

    def test_join_longer_single_char_sequence(self):
        """Handles longer sequences like full chapter titles."""
        assert clean_text("は\nじ\nめ\nに") == "はじめに"

    def test_normal_lines_preserved(self):
        """Multi-char lines separated by newlines are not collapsed."""
        text = "これは段落です\nこれも段落です"
        assert clean_text(text) == "これは段落です\nこれも段落です"

    def test_remove_decorative_symbols(self):
        """Decorative symbols are stripped."""
        assert clean_text("■ 第1章 概要") == "第1章 概要"
        assert clean_text("★重要なポイント★") == "重要なポイント"
        assert clean_text("●項目1") == "項目1"

    def test_collapse_excessive_newlines(self):
        """3+ consecutive newlines collapse to double newline."""
        assert clean_text("段落A\n\n\n\n段落B") == "段落A\n\n段落B"

    def test_double_newline_preserved(self):
        """Double newlines (paragraph boundaries) are kept."""
        assert clean_text("段落A\n\n段落B") == "段落A\n\n段落B"

    def test_blank_lines_with_spaces(self):
        """Lines containing only whitespace are normalized."""
        assert clean_text("行1\n   \n行2") == "行1\n\n行2"

    def test_fullwidth_space_lines(self):
        """Lines with only full-width spaces are normalized."""
        assert clean_text("行1\n\u3000\u3000\u3000\n行2") == "行1\n\n行2"

    def test_strip_per_line_whitespace(self):
        """Leading/trailing whitespace per line is stripped."""
        assert clean_text("  hello  \n  world  ") == "hello\nworld"

    def test_empty_string(self):
        """Empty input returns empty string."""
        assert clean_text("") == ""

    def test_only_decorative(self):
        """String of only decorative symbols returns empty."""
        assert clean_text("■●★") == ""

    def test_mixed_cleaning(self):
        """Multiple rules apply together."""
        text = "■ 第\n一\n章\n\n\n\n本文が始まります。"
        result = clean_text(text)
        assert result == "第一章\n\n本文が始まります。"

    def test_single_char_line_at_end(self):
        """Single char at end of sequence is properly joined."""
        assert clean_text("A\nB\nC") == "ABC"

    def test_no_false_join_on_multichar_lines(self):
        """Lines with 2+ chars should not be joined."""
        text = "AB\nCD\nEF"
        assert clean_text(text) == "AB\nCD\nEF"
