"""Refloom Python worker: staged probe / extract-pages / chunk commands."""

import json
import os
import sys
import time
import traceback
from pathlib import Path


def main():
    """Single-shot mode: read one JSON command from stdin, execute, exit."""
    try:
        request = json.loads(sys.stdin.read())
    except json.JSONDecodeError as e:
        _write_response({"status": "error", "error": f"Invalid JSON input: {e}"})
        return

    _dispatch_command(request)


def run_persistent():
    """Persistent mode: read newline-delimited JSON commands in a loop."""
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
        except json.JSONDecodeError as e:
            _write_response({"status": "error", "error": f"Invalid JSON input: {e}"})
            continue

        command = request.get("command")
        if command == "shutdown":
            break

        _dispatch_command(request)


def _dispatch_command(request: dict):
    command = request.get("command")
    try:
        if command == "probe":
            _handle_probe(request)
            return
        if command == "extract-pages":
            _handle_extract_pages(request)
            return
        if command == "chunk":
            _handle_chunk(request)
            return
    except Exception as e:  # pragma: no cover - defensive response wrapper
        _write_response({"status": "error", "error": f"Worker command failed: {e}"})
        print(traceback.format_exc(), file=sys.stderr)
        return

    _write_response({"status": "error", "error": f"Unknown command: {command}"})


def _handle_probe(request: dict):
    path = _require_existing_path(request.get("path"))
    fmt = _require_format(request.get("format"))

    if fmt == "pdf":
        from refloom_worker.pdf_extractor import probe_pdf

        result = probe_pdf(path)
    else:
        from refloom_worker.epub_extractor import probe_epub

        result = probe_epub(path)

    _write_response({"status": "ok", **result})


def _handle_extract_pages(request: dict):
    path = _require_existing_path(request.get("path"))
    fmt = _require_format(request.get("format"))
    output_path = _require_path_value(request.get("output_path"), "output_path")
    page_start = _require_positive_int(request.get("page_start"), "page_start")
    page_end = _require_positive_int(request.get("page_end"), "page_end")
    if page_end < page_start:
        raise ValueError("page_end must be greater than or equal to page_start")

    batch_start = time.perf_counter()
    if fmt == "pdf":
        from refloom_worker.pdf_extractor import extract_pdf_pages

        result = extract_pdf_pages(
            path, page_start, page_end,
            request.get("ocr_policy", "auto"), request.get("file_hash"),
        )
    else:
        from refloom_worker.epub_extractor import extract_epub_pages

        result = extract_epub_pages(path, page_start, page_end)

    _write_jsonl(result["pages"], output_path)
    _write_response({
        "status": "ok",
        "pages_written": len(result["pages"]),
        "stats": result.get("stats", {}),
        "batch_ms": round((time.perf_counter() - batch_start) * 1000),
    })


def _handle_chunk(request: dict):
    pages_path = _require_existing_path(request.get("pages_path"))
    chapters_path = _require_existing_path(request.get("chapters_path"))
    output_path = _require_path_value(request.get("output_path"), "output_path")
    fmt = _require_format(request.get("format"))
    options = request.get("options", {})
    chunk_size = _require_positive_int(options.get("chunk_size", 500), "chunk_size")
    chunk_overlap = _require_non_negative_int(options.get("chunk_overlap", 100), "chunk_overlap")

    from refloom_worker.chunker import chunk_pages, load_pages_jsonl, write_chunks_jsonl
    from refloom_worker.quality import classify_extraction

    chunk_start = time.perf_counter()
    pages = load_pages_jsonl(pages_path)
    chapters = json.loads(Path(chapters_path).read_text(encoding="utf-8"))
    quality = classify_extraction(pages)
    if fmt == "epub":
        pages, quality = _apply_epub_repair(pages, quality)

    chunks = chunk_pages(
        pages,
        chapters,
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
    )
    chunks_written = write_chunks_jsonl(chunks, output_path)
    _write_response({
        "status": "ok",
        "quality": quality,
        "chunks_written": chunks_written,
        "chunk_ms": round((time.perf_counter() - chunk_start) * 1000),
    })


def _apply_epub_repair(pages: list[dict], quality: str) -> tuple[list[dict], str]:
    if quality != "text_corrupt":
        return pages, quality

    from refloom_worker.epub_extractor import repair_pages
    from refloom_worker.quality import classify_extraction

    repaired_pages = repair_pages(pages)
    repaired_quality = classify_extraction(repaired_pages)
    if repaired_pages != pages:
        return repaired_pages, repaired_quality

    return pages, quality


def _write_jsonl(rows: list[dict], output_path: str):
    path = Path(output_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as fh:
        for row in rows:
            fh.write(json.dumps(row, ensure_ascii=False) + "\n")


def _require_existing_path(value: str | None) -> str:
    path = _require_path_value(value, "path")
    if not os.path.exists(path):
        raise FileNotFoundError(f"File not found: {path}")
    return path


def _require_path_value(value: str | None, name: str) -> str:
    if not value:
        raise ValueError(f"Missing required field: {name}")
    return value


def _require_format(value: str | None) -> str:
    if value not in ("pdf", "epub"):
        raise ValueError(f"Unsupported format: {value}")
    return value


def _require_positive_int(value: int | None, name: str) -> int:
    if not isinstance(value, int) or value <= 0:
        raise ValueError(f"{name} must be a positive integer")
    return value


def _require_non_negative_int(value: int | None, name: str) -> int:
    if not isinstance(value, int) or value < 0:
        raise ValueError(f"{name} must be a non-negative integer")
    return value


def _write_response(response: dict):
    """Write a JSON response line to stdout and flush immediately."""
    sys.stdout.write(json.dumps(response, ensure_ascii=False) + "\n")
    sys.stdout.flush()


if __name__ == "__main__":
    if "--persistent" in sys.argv:
        run_persistent()
    else:
        main()
