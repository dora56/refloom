"""PDF extraction using PyMuPDF (fitz) with optional Vision Framework OCR."""

import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path

import fitz

_FAST_RENDER_SCALE = 1.5
_ACCURATE_RENDER_SCALE = 2.0
_FAST_RECOGNITION_LEVEL = 1
_ACCURATE_RECOGNITION_LEVEL = 0
_OCR_RETRY_MIN_CHARS = 50
_OCR_FAST_SCALE_ENV = "REFLOOM_OCR_FAST_SCALE"
_OCR_RETRY_MIN_CHARS_ENV = "REFLOOM_OCR_RETRY_MIN_CHARS"

# LRU-1 document cache for persistent worker mode.
# Avoids re-opening the same PDF on consecutive batch calls.
_cached_doc: tuple[str, fitz.Document] | None = None


@dataclass(frozen=True)
class OCRSettings:
    fast_render_scale: float
    accurate_render_scale: float
    retry_min_chars: int


def probe_pdf(path: str) -> dict:
    """Return metadata and extraction guidance for a PDF."""
    with fitz.open(path) as doc:
        book, chapters = _book_and_chapters(doc, path)
        sample_pages = _sample_page_numbers(len(doc))
        sample_failures = 0
        for page_num in sample_pages:
            text = _page_text(doc[page_num - 1])
            if not text.strip():
                sample_failures += 1

    ocr_heavy = len(sample_pages) > 0 and sample_failures * 2 > len(sample_pages)
    extraction_mode = "ocr-heavy" if ocr_heavy else "text"
    recommended_batch_size = 16 if ocr_heavy else 64
    return {
        "book": book,
        "chapters": chapters,
        "extraction_mode": extraction_mode,
        "recommended_batch_size": recommended_batch_size,
        "ocr_candidate_pages_estimate": sample_failures,
    }


def _get_cached_doc(path: str) -> fitz.Document:
    """Return a cached fitz.Document, opening a new one if the path changed."""
    global _cached_doc  # noqa: PLW0603
    if _cached_doc is not None and _cached_doc[0] == path:
        return _cached_doc[1]
    if _cached_doc is not None:
        _cached_doc[1].close()
    doc = fitz.open(path)
    _cached_doc = (path, doc)
    return doc


def close_cached_doc():
    """Close and discard the cached document. Called on worker shutdown."""
    global _cached_doc  # noqa: PLW0603
    if _cached_doc is not None:
        _cached_doc[1].close()
        _cached_doc = None


def _ocr_cache_dir() -> Path:
    return Path.home() / ".refloom" / "cache" / "ocr"


def _ocr_cache_key(file_hash: str, page_num: int, render_scale: float, recognition_level: int) -> str:
    raw = f"{file_hash}:{page_num}:{render_scale}:{recognition_level}"
    return hashlib.sha256(raw.encode()).hexdigest()


def _ocr_cache_get(file_hash: str | None, page_num: int, render_scale: float, recognition_level: int) -> str | None:
    if not file_hash:
        return None
    key = _ocr_cache_key(file_hash, page_num, render_scale, recognition_level)
    path = _ocr_cache_dir() / f"{key}.json"
    if path.exists():
        try:
            return json.loads(path.read_text(encoding="utf-8"))["text"]
        except (json.JSONDecodeError, KeyError):
            return None
    return None


def _ocr_cache_put(file_hash: str | None, page_num: int, render_scale: float, recognition_level: int, text: str):
    if not file_hash:
        return
    cache_dir = _ocr_cache_dir()
    cache_dir.mkdir(parents=True, exist_ok=True)
    key = _ocr_cache_key(file_hash, page_num, render_scale, recognition_level)
    path = cache_dir / f"{key}.json"
    path.write_text(json.dumps({"text": text}, ensure_ascii=False), encoding="utf-8")


def extract_pdf_pages(
    path: str, page_start: int, page_end: int,
    ocr_policy: str = "auto", file_hash: str | None = None,
) -> dict:
    """Extract a bounded PDF page range with OCR fallback."""
    doc = _get_cached_doc(path)
    page_end = min(page_end, len(doc))
    ocr_available = _check_ocr_available()
    ocr_settings = _load_ocr_settings()
    stats = {
        "ocr_pages": 0,
        "ocr_retries": 0,
        "ocr_ms": 0,
        "ocr_fast_pages": 0,
        "ocr_retry_pages": 0,
        "ocr_fast_ms": 0,
        "ocr_retry_ms": 0,
    }
    pages = []
    for page_num in range(page_start, page_end + 1):
        page = doc[page_num - 1]
        text = _page_text(page)
        should_try_ocr = ocr_policy != "never" and not text.strip() and ocr_available
        if should_try_ocr:
            stats["ocr_pages"] += 1
            if ocr_policy == "accurate-only":
                # Skip fast pass; go straight to accurate OCR
                cached = _ocr_cache_get(
                    file_hash, page_num, ocr_settings.accurate_render_scale, _ACCURATE_RECOGNITION_LEVEL,
                )
                if cached is not None:
                    text = cached
                    stats.setdefault("ocr_cache_hits", 0)
                    stats["ocr_cache_hits"] += 1
                else:
                    start = time.perf_counter()
                    text = _ocr_page(
                        page,
                        render_scale=ocr_settings.accurate_render_scale,
                        recognition_level=_ACCURATE_RECOGNITION_LEVEL,
                    )
                    stats["ocr_ms"] += _elapsed_ms(start)
                    _ocr_cache_put(
                        file_hash, page_num, ocr_settings.accurate_render_scale, _ACCURATE_RECOGNITION_LEVEL, text,
                    )
            else:
                # Default auto: fast pass, then retry with accurate if needed
                stats["ocr_fast_pages"] += 1
                fast_start = time.perf_counter()
                fast_text = _ocr_page(
                    page,
                    render_scale=ocr_settings.fast_render_scale,
                    recognition_level=_FAST_RECOGNITION_LEVEL,
                )
                fast_elapsed_ms = _elapsed_ms(fast_start)
                stats["ocr_fast_ms"] += fast_elapsed_ms
                stats["ocr_ms"] += fast_elapsed_ms
                text = fast_text
                if _non_space_len(fast_text) < ocr_settings.retry_min_chars:
                    stats["ocr_retries"] += 1
                    stats["ocr_retry_pages"] += 1
                    retry_start = time.perf_counter()
                    accurate_text = _ocr_page(
                        page,
                        render_scale=ocr_settings.accurate_render_scale,
                        recognition_level=_ACCURATE_RECOGNITION_LEVEL,
                    )
                    retry_elapsed_ms = _elapsed_ms(retry_start)
                    stats["ocr_retry_ms"] += retry_elapsed_ms
                    stats["ocr_ms"] += retry_elapsed_ms
                    text = _preferred_ocr_text(fast_text, accurate_text)

        pages.append({"page_num": page_num, "text": text})

    return {"pages": pages, "stats": stats}


def extract_pdf(path: str) -> dict:
    """Backward-compatible full-document extraction used by legacy tests/tools."""
    probe = probe_pdf(path)
    extracted = extract_pdf_pages(path, 1, probe["book"]["page_count"])
    return {
        "book": probe["book"],
        "chapters": probe["chapters"],
        "pages": extracted["pages"],
        "stats": extracted["stats"],
    }


def _filename_title(path: str) -> str:
    """Derive title from filename."""
    import os
    name = os.path.basename(path)
    name = os.path.splitext(name)[0]
    return name


def _build_chapters(toc: list, total_pages: int) -> list:
    """Build chapter list from TOC entries.

    Uses only level-1 TOC entries as chapters.
    """
    # Filter to level-1 entries only
    level1 = [(title, page) for level, title, page in toc if level == 1]
    if not level1:
        return []

    chapters = []
    for i, (title, page_start) in enumerate(level1):
        if i + 1 < len(level1):
            page_end = level1[i + 1][1] - 1
        else:
            page_end = total_pages
        chapters.append({
            "title": title,
            "order": i,
            "page_start": page_start,
            "page_end": page_end,
        })

    return chapters


def _book_and_chapters(doc, path: str) -> tuple[dict, list]:
    metadata = doc.metadata or {}
    title = metadata.get("title", "") or _filename_title(path)
    author = metadata.get("author", "")
    toc = doc.get_toc(simple=True)
    chapters = _build_chapters(toc, len(doc))
    if not chapters:
        chapters = [{"title": "全体", "order": 0, "page_start": 1, "page_end": len(doc)}]
    return {
        "title": title,
        "author": author,
        "format": "pdf",
        "page_count": len(doc),
    }, chapters


def _sample_page_numbers(page_count: int) -> list[int]:
    if page_count <= 0:
        return []
    candidates = [1, max(1, (page_count + 1) // 2), page_count]
    result = []
    for page_num in candidates:
        if page_num not in result:
            result.append(page_num)
    return result


def _page_text(page) -> str:
    raw_text = page.get_text("text")
    if isinstance(raw_text, str):
        return raw_text
    return ""


def _check_ocr_available() -> bool:
    """Check if Vision Framework OCR is available."""
    if sys.platform != "darwin":
        return False
    try:
        from refloom_worker.ocr_vision import is_available
        return is_available()
    except ImportError:
        return False


def _load_ocr_settings() -> OCRSettings:
    return OCRSettings(
        fast_render_scale=_env_float(_OCR_FAST_SCALE_ENV, _FAST_RENDER_SCALE),
        accurate_render_scale=_ACCURATE_RENDER_SCALE,
        retry_min_chars=_env_int(_OCR_RETRY_MIN_CHARS_ENV, _OCR_RETRY_MIN_CHARS),
    )


def _ocr_page(page, render_scale: float, recognition_level: int) -> str:
    """OCR a single PDF page using Vision Framework.

    Renders the page to a high-resolution PNG, then passes to Vision OCR.
    """
    try:
        from refloom_worker.ocr_vision import recognize_text

        pix = page.get_pixmap(matrix=fitz.Matrix(render_scale, render_scale), alpha=False)
        image_data = pix.tobytes(output="png")

        texts = recognize_text(image_data, recognition_level=recognition_level)
        return "\n".join(texts)
    except Exception:
        return ""


def _preferred_ocr_text(fast_text: str, accurate_text: str) -> str:
    fast_len = _non_space_len(fast_text)
    accurate_len = _non_space_len(accurate_text)
    if accurate_len > fast_len:
        return accurate_text
    return fast_text


def _non_space_len(text: str) -> int:
    return sum(1 for char in text if not char.isspace())


def _env_float(name: str, default: float) -> float:
    value = os.environ.get(name)
    if value is None:
        return default
    try:
        return float(value)
    except ValueError:
        return default


def _env_int(name: str, default: int) -> int:
    value = os.environ.get(name)
    if value is None:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def _elapsed_ms(start: float) -> int:
    return round((time.perf_counter() - start) * 1000)
