"""Refloom Python worker: reads JSON from stdin, extracts and chunks documents, writes JSON to stdout."""

import json
import os
import sys
import traceback


def main():
    try:
        request = json.loads(sys.stdin.read())
    except json.JSONDecodeError as e:
        _error_response(f"Invalid JSON input: {e}")
        return

    command = request.get("command")
    if command != "extract":
        _error_response(f"Unknown command: {command}")
        return

    path = request.get("path")
    fmt = request.get("format")
    options = request.get("options", {})

    if not path or not os.path.exists(path):
        _error_response(f"File not found: {path}")
        return

    if fmt not in ("pdf", "epub"):
        _error_response(f"Unsupported format: {fmt}")
        return

    chunk_size = options.get("chunk_size", 500)
    chunk_overlap = options.get("chunk_overlap", 100)

    try:
        if fmt == "pdf":
            from refloom_worker.pdf_extractor import extract_pdf
            result = extract_pdf(path)
        else:
            from refloom_worker.epub_extractor import extract_epub
            result = extract_epub(path)

        # Classify extraction quality
        from refloom_worker.quality import classify_extraction
        quality = classify_extraction(result["pages"])

        # Chunk the extracted pages
        from refloom_worker.chunker import chunk_pages
        chunks = chunk_pages(
            result["pages"],
            result["chapters"],
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
        )

        # Build response (exclude raw pages to keep response smaller)
        response = {
            "status": "ok",
            "quality": quality,
            "book": result["book"],
            "chapters": result["chapters"],
            "chunks": chunks,
        }

        print(json.dumps(response, ensure_ascii=False))

    except Exception as e:
        _error_response(f"Extraction failed: {e}", traceback.format_exc())


def _error_response(error: str, details: str = ""):
    response = {"status": "error", "error": error, "details": details}
    print(json.dumps(response, ensure_ascii=False))
    print(details, file=sys.stderr)


if __name__ == "__main__":
    main()
