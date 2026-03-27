"""Tests for EPUB text cleaning and repair logic."""

from refloom_worker.epub_extractor import (
    _build_chapters_from_toc,
    clean_text,
    extract_epub_pages,
    probe_epub,
    repair_pages,
    repair_text,
)


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


class TestRepairText:
    """Tests for conservative EPUB repair."""

    def test_removes_control_and_replacement_chars(self):
        repaired, changed = repair_text("正常\u200bテキスト\ufffd\u0000")
        assert changed is True
        assert repaired == "正常テキスト"

    def test_keeps_text_when_repair_would_shrink_too_much(self):
        text = "\ufffd\ufffd\ufffd"
        repaired, changed = repair_text(text)
        assert changed is False
        assert repaired == text

    def test_repairs_pages_in_place(self):
        pages = [{"page_num": 1, "text": "A\u200bB\ufffd"}]
        repaired = repair_pages(pages)
        assert repaired == [{"page_num": 1, "text": "AB"}]


def _make_test_epub(tmp_path, title="Test Book", author="Author", chapters=None):
    """Create a minimal EPUB file for testing."""
    from ebooklib import epub

    book = epub.EpubBook()
    book.set_identifier("test-id-123")
    book.set_title(title)
    book.set_language("ja")
    book.add_author(author)

    if chapters is None:
        chapters = [("Chapter 1", "<p>First chapter content</p>")]

    spine_items: list = ["nav"]
    toc = []
    for i, (ch_title, html_body) in enumerate(chapters):
        ch = epub.EpubHtml(
            title=ch_title,
            file_name=f"ch{i}.xhtml",
            lang="ja",
        )
        ch.content = f"<html><body><h1>{ch_title}</h1>{html_body}</body></html>".encode()
        book.add_item(ch)
        spine_items.append(ch)
        toc.append(epub.Link(f"ch{i}.xhtml", ch_title, f"ch{i}"))

    book.toc = toc
    book.add_item(epub.EpubNcx())
    book.add_item(epub.EpubNav())
    book.spine = spine_items

    path = str(tmp_path / "test.epub")
    epub.write_epub(path, book)
    return path


def test_probe_epub_returns_book_metadata(tmp_path):
    path = _make_test_epub(tmp_path, title="My Book", author="Taro")
    result = probe_epub(path)
    assert result["book"]["title"] == "My Book"
    assert result["book"]["author"] == "Taro"
    assert result["book"]["format"] == "epub"
    assert result["book"]["page_count"] >= 1
    assert result["extraction_mode"] == "text"
    assert len(result["chapters"]) >= 1


def test_probe_epub_multiple_chapters(tmp_path):
    chapters = [
        ("第一章", "<p>内容A</p>"),
        ("第二章", "<p>内容B</p>"),
    ]
    path = _make_test_epub(tmp_path, chapters=chapters)
    result = probe_epub(path)
    assert len(result["chapters"]) == 2
    assert result["chapters"][0]["title"] == "第一章"
    assert result["chapters"][1]["title"] == "第二章"


def test_extract_epub_pages_returns_text(tmp_path):
    chapters = [("Ch1", "<p>Hello world</p>")]
    path = _make_test_epub(tmp_path, chapters=chapters)
    result = extract_epub_pages(path, 1, 10)
    assert len(result["pages"]) >= 1
    # At least one page should contain "Hello world"
    texts = [p["text"] for p in result["pages"]]
    assert any("Hello world" in t for t in texts)
    assert result["stats"]["ocr_pages"] == 0


def test_extract_epub_pages_respects_range(tmp_path):
    chapters = [
        ("Ch1", "<p>Page one</p>"),
        ("Ch2", "<p>Page two</p>"),
        ("Ch3", "<p>Page three</p>"),
    ]
    path = _make_test_epub(tmp_path, chapters=chapters)
    # Extract only first page
    result = extract_epub_pages(path, 1, 1)
    assert len(result["pages"]) == 1
    assert result["pages"][0]["page_num"] == 1


def test_build_chapters_from_toc_spreads_ranges_across_pages():
    chapters = _build_chapters_from_toc([("第1章", "a"), ("第2章", "b")], 10)

    assert chapters == [
        {"title": "第1章", "order": 0, "page_start": 1, "page_end": 5},
        {"title": "第2章", "order": 1, "page_start": 6, "page_end": 10},
    ]
