"""Tests for the worker entrypoint command handlers."""

import json
from pathlib import Path

import pytest

from refloom_worker.main import (
    _apply_epub_repair,
    _handle_chunk,
    _handle_extract_pages,
    _handle_probe,
    run_persistent,
)

# --- EPUB repair tests (existing) ---


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


# --- _handle_probe tests ---


def test_handle_probe_pdf(monkeypatch, capsys):
    monkeypatch.setattr(
        "refloom_worker.pdf_extractor.probe_pdf",
        lambda path: {"total_pages": 10, "mode": "ocr-heavy", "batch_size": 16},
    )

    _handle_probe({"path": __file__, "format": "pdf"})

    out = json.loads(capsys.readouterr().out)
    assert out["status"] == "ok"
    assert out["total_pages"] == 10
    assert out["mode"] == "ocr-heavy"


# --- _handle_extract_pages tests ---


def test_handle_extract_pages_pdf(monkeypatch, capsys, tmp_path):
    output_path = str(tmp_path / "pages.jsonl")
    mock_pages = [
        {"page_num": 1, "text": "Page 1 content"},
        {"page_num": 2, "text": "Page 2 content"},
    ]

    monkeypatch.setattr(
        "refloom_worker.pdf_extractor.extract_pdf_pages",
        lambda path, start, end, ocr_policy: {"pages": mock_pages, "stats": {"ocr_pages": 0}},
    )

    _handle_extract_pages({
        "path": __file__,
        "format": "pdf",
        "output_path": output_path,
        "page_start": 1,
        "page_end": 2,
        "ocr_policy": "auto",
    })

    out = json.loads(capsys.readouterr().out)
    assert out["status"] == "ok"
    assert out["pages_written"] == 2
    assert out["stats"]["ocr_pages"] == 0
    assert out["batch_ms"] >= 0

    # Verify JSONL file was written
    lines = Path(output_path).read_text().strip().split("\n")
    assert len(lines) == 2
    assert json.loads(lines[0])["page_num"] == 1


def test_handle_extract_pages_epub(monkeypatch, capsys, tmp_path):
    output_path = str(tmp_path / "pages.jsonl")

    monkeypatch.setattr(
        "refloom_worker.epub_extractor.extract_epub_pages",
        lambda path, start, end: {"pages": [{"page_num": 1, "text": "Chapter 1"}], "stats": {}},
    )

    _handle_extract_pages({
        "path": __file__,
        "format": "epub",
        "output_path": output_path,
        "page_start": 1,
        "page_end": 1,
    })

    out = json.loads(capsys.readouterr().out)
    assert out["status"] == "ok"
    assert out["pages_written"] == 1


def test_handle_extract_pages_invalid_page_range():
    with pytest.raises(ValueError, match="page_end must be greater"):
        _handle_extract_pages({
            "path": __file__,
            "format": "pdf",
            "output_path": "/tmp/out.jsonl",
            "page_start": 10,
            "page_end": 5,
        })


def test_handle_extract_pages_missing_path():
    with pytest.raises(ValueError, match="Missing required field"):
        _handle_extract_pages({
            "format": "pdf",
            "output_path": "/tmp/out.jsonl",
            "page_start": 1,
            "page_end": 1,
        })


# --- _handle_chunk tests ---


def test_handle_chunk(monkeypatch, capsys, tmp_path):
    # Create temp pages and chapters files
    pages_path = tmp_path / "pages.jsonl"
    pages_path.write_text('{"page_num":1,"text":"Hello world"}\n')

    chapters_path = tmp_path / "chapters.json"
    chapters_path.write_text('[{"title":"Ch1","page_start":1,"page_end":1}]')

    output_path = str(tmp_path / "chunks.jsonl")

    mock_pages = [{"page_num": 1, "text": "Hello world"}]
    mock_chunks = [{"chunk_id": 1, "body": "Hello world", "heading": "Ch1"}]

    monkeypatch.setattr(
        "refloom_worker.chunker.load_pages_jsonl",
        lambda path: mock_pages,
    )
    monkeypatch.setattr(
        "refloom_worker.quality.classify_extraction",
        lambda pages: "ok",
    )
    monkeypatch.setattr(
        "refloom_worker.chunker.chunk_pages",
        lambda pages, chapters, chunk_size=500, chunk_overlap=100: mock_chunks,
    )
    monkeypatch.setattr(
        "refloom_worker.chunker.write_chunks_jsonl",
        lambda chunks, path: len(chunks),
    )

    _handle_chunk({
        "pages_path": str(pages_path),
        "chapters_path": str(chapters_path),
        "output_path": output_path,
        "format": "pdf",
        "options": {"chunk_size": 500, "chunk_overlap": 100},
    })

    out = json.loads(capsys.readouterr().out)
    assert out["status"] == "ok"
    assert out["quality"] == "ok"
    assert out["chunks_written"] == 1
    assert out["chunk_ms"] >= 0


def test_handle_chunk_missing_pages_path():
    with pytest.raises(ValueError, match="Missing required field"):
        _handle_chunk({
            "chapters_path": __file__,
            "output_path": "/tmp/out.jsonl",
            "format": "pdf",
        })


# --- Persistent mode tests ---


def test_run_persistent_processes_multiple_commands(monkeypatch, capsys):
    monkeypatch.setattr(
        "refloom_worker.pdf_extractor.probe_pdf",
        lambda path: {"total_pages": 5, "mode": "text"},
    )

    commands = [
        json.dumps({"command": "probe", "path": __file__, "format": "pdf"}),
        json.dumps({"command": "probe", "path": __file__, "format": "pdf"}),
        json.dumps({"command": "shutdown"}),
    ]
    monkeypatch.setattr("sys.stdin", iter(line + "\n" for line in commands))

    run_persistent()

    lines = capsys.readouterr().out.strip().split("\n")
    assert len(lines) == 2
    for line in lines:
        resp = json.loads(line)
        assert resp["status"] == "ok"
        assert resp["total_pages"] == 5


def test_run_persistent_handles_invalid_json(monkeypatch, capsys):
    commands = [
        "not valid json",
        json.dumps({"command": "shutdown"}),
    ]
    monkeypatch.setattr("sys.stdin", iter(line + "\n" for line in commands))

    run_persistent()

    lines = capsys.readouterr().out.strip().split("\n")
    assert len(lines) == 1
    resp = json.loads(lines[0])
    assert resp["status"] == "error"
    assert "Invalid JSON" in resp["error"]


def test_run_persistent_handles_eof(monkeypatch, capsys):
    monkeypatch.setattr(
        "refloom_worker.pdf_extractor.probe_pdf",
        lambda path: {"total_pages": 1},
    )

    commands = [
        json.dumps({"command": "probe", "path": __file__, "format": "pdf"}),
        # No shutdown — just EOF
    ]
    monkeypatch.setattr("sys.stdin", iter(line + "\n" for line in commands))

    run_persistent()

    lines = capsys.readouterr().out.strip().split("\n")
    assert len(lines) == 1
    assert json.loads(lines[0])["status"] == "ok"


def test_run_persistent_skips_blank_lines(monkeypatch, capsys):
    monkeypatch.setattr(
        "refloom_worker.pdf_extractor.probe_pdf",
        lambda path: {"total_pages": 1},
    )

    commands = [
        "",
        "  ",
        json.dumps({"command": "probe", "path": __file__, "format": "pdf"}),
        json.dumps({"command": "shutdown"}),
    ]
    monkeypatch.setattr("sys.stdin", iter(line + "\n" for line in commands))

    run_persistent()

    lines = capsys.readouterr().out.strip().split("\n")
    assert len(lines) == 1
    assert json.loads(lines[0])["status"] == "ok"
