"""Tests for EPUB repair application in the worker entrypoint."""

from refloom_worker.main import _apply_epub_repair


def test_apply_epub_repair_keeps_improved_pages_even_if_quality_stays_text_corrupt():
    pages = [
        {"page_num": 1, "text": "A\u200bB"},
        {"page_num": 2, "text": "\ufffd\ufffd\ufffd"},
        {"page_num": 3, "text": "\ufffd\ufffd\ufffd"},
        {"page_num": 4, "text": "\ufffd\ufffd\ufffd"},
    ]

    repaired_pages, repaired_quality = _apply_epub_repair(pages, "text_corrupt")

    assert repaired_pages[0]["text"] == "AB"
    assert repaired_pages[1:] == pages[1:]
    assert repaired_quality == "text_corrupt"


def test_apply_epub_repair_skips_non_corrupt_books():
    pages = [{"page_num": 1, "text": "正常な本文です"}]

    repaired_pages, repaired_quality = _apply_epub_repair(pages, "ok")

    assert repaired_pages == pages
    assert repaired_quality == "ok"
