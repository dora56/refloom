"""Tests for chunker module."""

import json

from refloom_worker.chunker import (
    _assemble_chunks,
    _force_split,
    _split_paragraphs,
    chunk_pages,
    load_pages_jsonl,
    write_chunks_jsonl,
)

# --- chunk_pages ---


def test_chunk_pages_empty_input():
    assert chunk_pages([], [], chunk_size=500) == []


def test_chunk_pages_single_page_single_chapter():
    pages = [{"page_num": 1, "text": "Hello world"}]
    chapters = [{"title": "Ch1", "order": 0, "page_start": 1, "page_end": 1}]
    result = chunk_pages(pages, chapters, chunk_size=500)
    assert len(result) == 1
    assert result[0]["heading"] == "Ch1"
    assert result[0]["body"] == "Hello world"
    assert result[0]["chapter_order"] == 0
    assert result[0]["chunk_order"] == 0
    assert result[0]["page_start"] == 1
    assert result[0]["page_end"] == 1


def test_chunk_pages_skips_chapter_with_no_pages():
    pages = [{"page_num": 1, "text": "Only page"}]
    chapters = [
        {"title": "Ch1", "order": 0, "page_start": 1, "page_end": 1},
        {"title": "Ch2", "order": 1, "page_start": 10, "page_end": 20},
    ]
    result = chunk_pages(pages, chapters, chunk_size=500)
    assert len(result) == 1
    assert result[0]["heading"] == "Ch1"


def test_chunk_pages_multiple_chapters():
    pages = [
        {"page_num": 1, "text": "Page one text"},
        {"page_num": 2, "text": "Page two text"},
        {"page_num": 3, "text": "Page three text"},
    ]
    chapters = [
        {"title": "Part A", "order": 0, "page_start": 1, "page_end": 2},
        {"title": "Part B", "order": 1, "page_start": 3, "page_end": 3},
    ]
    result = chunk_pages(pages, chapters, chunk_size=500)
    assert len(result) == 2
    assert result[0]["heading"] == "Part A"
    assert result[1]["heading"] == "Part B"


def test_chunk_pages_splits_large_chapter():
    text = "A" * 300
    pages = [{"page_num": 1, "text": text + "\n\n" + text}]
    chapters = [{"title": "Big", "order": 0, "page_start": 1, "page_end": 1}]
    result = chunk_pages(pages, chapters, chunk_size=400, chunk_overlap=50)
    assert len(result) >= 2
    # All chunks should belong to the same chapter
    for c in result:
        assert c["heading"] == "Big"
        assert c["chapter_order"] == 0


def test_chunk_pages_japanese_text():
    text = "これはテストです。" * 50  # 450 chars
    pages = [{"page_num": 1, "text": text + "\n\n" + text}]
    chapters = [{"title": "日本語章", "order": 0, "page_start": 1, "page_end": 1}]
    result = chunk_pages(pages, chapters, chunk_size=500, chunk_overlap=50)
    assert len(result) >= 1
    assert result[0]["heading"] == "日本語章"


# --- _split_paragraphs ---


def test_split_paragraphs_double_newline():
    result = _split_paragraphs("Para one.\n\nPara two.")
    assert result == ["Para one.", "Para two."]


def test_split_paragraphs_triple_newline():
    result = _split_paragraphs("A\n\n\nB")
    assert result == ["A", "B"]


def test_split_paragraphs_empty():
    assert _split_paragraphs("") == []


def test_split_paragraphs_single_paragraph():
    assert _split_paragraphs("No breaks here") == ["No breaks here"]


# --- _assemble_chunks ---


def test_assemble_chunks_empty_paragraphs():
    assert _assemble_chunks([], 500, 100, "H", 0, 1, 1) == []


def test_assemble_chunks_single_small_paragraph():
    result = _assemble_chunks(["short text"], 500, 100, "H", 0, 1, 5)
    assert len(result) == 1
    assert result[0]["body"] == "short text"
    assert result[0]["page_start"] == 1
    assert result[0]["page_end"] == 5


def test_assemble_chunks_overlap_present():
    paragraphs = ["A" * 300, "B" * 300]
    result = _assemble_chunks(paragraphs, 400, 50, "H", 0, 1, 1)
    assert len(result) == 2
    # Second chunk should contain overlap from first
    assert result[1]["chunk_order"] == 1


def test_assemble_chunks_chunk_order_increments():
    paragraphs = ["X" * 200, "Y" * 200, "Z" * 200]
    result = _assemble_chunks(paragraphs, 250, 50, "H", 0, 1, 1)
    orders = [c["chunk_order"] for c in result]
    assert orders == list(range(len(result)))


# --- _force_split ---


def test_force_split_at_japanese_period():
    text = "第一文。第二文。第三文。第四文。第五文。"
    parts = _force_split(text, 10)
    assert len(parts) >= 2
    # All text should be preserved
    assert "".join(parts) == text


def test_force_split_no_split_needed():
    text = "Short text"
    parts = _force_split(text, 500)
    assert parts == ["Short text"]


def test_force_split_returns_original_if_no_boundary():
    text = "A" * 100
    parts = _force_split(text, 30)
    # No sentence boundary, returns as-is
    assert parts == [text]


# --- load_pages_jsonl / write_chunks_jsonl ---


def test_load_write_roundtrip(tmp_path):
    pages_path = tmp_path / "pages.jsonl"
    pages = [
        {"page_num": 1, "text": "Hello"},
        {"page_num": 2, "text": "World"},
    ]
    with pages_path.open("w", encoding="utf-8") as fh:
        for p in pages:
            fh.write(json.dumps(p, ensure_ascii=False) + "\n")

    loaded = load_pages_jsonl(str(pages_path))
    assert len(loaded) == 2
    assert loaded[0]["text"] == "Hello"


def test_write_chunks_jsonl_creates_parent_dirs(tmp_path):
    output_path = tmp_path / "sub" / "chunks.jsonl"
    chunks = [{"body": "test", "chunk_order": 0}]
    count = write_chunks_jsonl(chunks, str(output_path))
    assert count == 1
    assert output_path.exists()

    loaded = json.loads(output_path.read_text(encoding="utf-8").strip())
    assert loaded["body"] == "test"


def test_load_pages_jsonl_skips_blank_lines(tmp_path):
    path = tmp_path / "pages.jsonl"
    path.write_text('{"page_num": 1, "text": "a"}\n\n{"page_num": 2, "text": "b"}\n')
    loaded = load_pages_jsonl(str(path))
    assert len(loaded) == 2
