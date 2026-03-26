"""EPUB extraction using ebooklib and BeautifulSoup."""

import re
import unicodedata

import ebooklib
from bs4 import BeautifulSoup
from ebooklib import epub

# Decorative symbols to strip (headings, bullets, section markers)
_DECORATIVE_RE = re.compile(r"[■□●○◆◇▲△▼▽★☆※◎▪▫►▻◄◅]")

# Consecutive single-char lines: e.g. "第\n一\n章" → "第一章"
# Matches sequences of (single char + newline) followed by a single char
_SINGLE_CHAR_LINES_RE = re.compile(r"(?:(.)\n(?=.\n|.$))")

# Three or more consecutive newlines → two newlines
_MULTI_NEWLINES_RE = re.compile(r"\n{3,}")

# Lines that are only whitespace (full-width or half-width spaces)
_BLANK_LINE_RE = re.compile(r"^[\s　]+$", re.MULTILINE)


def clean_text(text: str) -> str:
    """Clean extracted EPUB text by removing layout artifacts.

    Rules applied in order:
    1. Remove decorative symbols (■●★ etc.)
    2. Join consecutive single-character lines (layout-split titles)
    3. Normalize blank-only lines to empty lines
    4. Collapse 3+ consecutive newlines to 2
    5. Strip leading/trailing whitespace per line
    """
    # 1. Remove decorative symbols
    text = _DECORATIVE_RE.sub("", text)

    # 2. Join consecutive single-character lines
    # Repeatedly apply until stable (handles long runs like 5+ chars)
    prev = None
    while prev != text:
        prev = text
        text = _SINGLE_CHAR_LINES_RE.sub(r"\1", text)

    # 3. Normalize blank-only lines
    text = _BLANK_LINE_RE.sub("", text)

    # 4. Collapse excessive newlines
    text = _MULTI_NEWLINES_RE.sub("\n\n", text)

    # 5. Strip per-line whitespace
    lines = [line.strip() for line in text.split("\n")]
    text = "\n".join(lines)

    return text.strip()


def extract_epub(path: str) -> dict:
    """Extract text and structure from an EPUB file.

    Returns a dict with:
      - book: {title, author, format, page_count}
      - chapters: [{title, order, page_start, page_end}]
      - pages: [{page_num, text}]  (page_num is spine item index)
    """
    probe = probe_epub(path)
    extracted = extract_epub_pages(path, 1, probe["book"]["page_count"])
    return {
        "book": probe["book"],
        "chapters": probe["chapters"],
        "pages": extracted["pages"],
        "stats": extracted["stats"],
    }


def probe_epub(path: str) -> dict:
    book = epub.read_epub(path, options={"ignore_ncx": False})
    title, author = _book_metadata(path, book)
    toc_items = _extract_toc(book)
    ordered_items = _ordered_spine_items(book)
    page_count = len(ordered_items)
    chapters = _build_chapters_from_toc(toc_items, page_count)
    if not chapters:
        chapters = [{"title": "全体", "order": 0, "page_start": 1, "page_end": page_count}]
    return {
        "book": {
            "title": title,
            "author": author,
            "format": "epub",
            "page_count": page_count,
        },
        "chapters": chapters,
        "extraction_mode": "text",
        "recommended_batch_size": 64,
        "ocr_candidate_pages_estimate": 0,
    }


def extract_epub_pages(path: str, page_start: int, page_end: int) -> dict:
    book = epub.read_epub(path, options={"ignore_ncx": False})
    ordered_items = _ordered_spine_items(book)
    page_end = min(page_end, len(ordered_items))
    pages = []
    for i in range(page_start - 1, page_end):
        item = ordered_items[i]
        html = item.get_content().decode("utf-8", errors="replace")
        soup = BeautifulSoup(html, "html.parser")
        text = soup.get_text(separator="\n", strip=True)
        text = clean_text(text)
        pages.append({"page_num": i + 1, "text": text})

    return {
        "pages": pages,
        "stats": {
            "ocr_pages": 0,
            "ocr_retries": 0,
            "ocr_ms": 0,
            "ocr_fast_pages": 0,
            "ocr_retry_pages": 0,
            "ocr_fast_ms": 0,
            "ocr_retry_ms": 0,
        },
    }


def repair_pages(pages: list[dict]) -> list[dict]:
    """Repair suspicious EPUB text while preserving page order and count."""
    repaired_pages = []
    for page in pages:
        text = page.get("text", "")
        repaired_text, _ = repair_text(text)
        repaired_pages.append({
            "page_num": page.get("page_num"),
            "text": repaired_text,
        })
    return repaired_pages


def repair_text(text: str) -> tuple[str, bool]:
    """Attempt conservative text repair for control/replacement-character damage."""
    if not text:
        return text, False

    original_ratio, original_printable, original_suspicious = _quality_metrics(text)

    candidate = unicodedata.normalize("NFKC", text)
    candidate = candidate.replace("\r\n", "\n").replace("\r", "\n")
    candidate = candidate.replace("\u00a0", " ").replace("\u200b", "")
    candidate = candidate.replace("\ufeff", "").replace("\ufffd", "")
    candidate = "".join(_repair_char(char) for char in candidate)
    candidate = clean_text(candidate)

    candidate_ratio, candidate_printable, _candidate_suspicious = _quality_metrics(candidate)
    if candidate == text:
        return text, False
    if candidate_ratio >= original_ratio:
        return text, False
    if original_printable > 0 and candidate_printable == 0:
        return text, False
    min_printable = max(0, original_printable - original_suspicious)
    if candidate_printable < min_printable:
        return text, False
    return candidate, True


def _get_metadata(book, field: str) -> str:
    values = book.get_metadata("DC", field)
    if values:
        return values[0][0]
    return ""


def _book_metadata(path: str, book) -> tuple[str, str]:
    raw_title = _get_metadata(book, "title")
    placeholder_titles = ("unknown", "untitled", "")
    if raw_title and "(blank)" not in raw_title.lower() and raw_title.strip().lower() not in placeholder_titles:
        title = raw_title
    else:
        title = _filename_title(path)
    author = _get_metadata(book, "creator") or ""
    return title, author


def _filename_title(path: str) -> str:
    import os
    name = os.path.basename(path)
    name = os.path.splitext(name)[0]
    return name


def _extract_toc(book) -> list:
    """Extract flat list of TOC entries: [(title, href), ...]."""
    result = []
    toc = book.toc
    if not toc:
        return result
    _flatten_toc(toc, result)
    return result


def _ordered_spine_items(book) -> list:
    spine_items = list(book.get_items_of_type(ebooklib.ITEM_DOCUMENT))
    spine_ids = [item_id for item_id, _ in book.spine]
    ordered_items = []
    id_to_item = {item.get_id(): item for item in spine_items}
    for sid in spine_ids:
        if sid in id_to_item:
            ordered_items.append(id_to_item[sid])

    if not ordered_items:
        ordered_items = spine_items
    return ordered_items


def _flatten_toc(items, result):
    """Recursively flatten TOC structure."""
    for item in items:
        if isinstance(item, tuple) and len(item) == 2:
            # (Section, [children])
            section, children = item
            if hasattr(section, "title"):
                result.append((section.title, getattr(section, "href", "")))
            _flatten_toc(children, result)
        elif hasattr(item, "title"):
            # ebooklib.epub.Link
            result.append((item.title, getattr(item, "href", "")))


def _build_chapters_from_toc(toc_items: list, total_pages: int) -> list:
    """Build chapters from TOC. Assign sequential order; page info is approximate."""
    if not toc_items:
        return []

    chapters = []
    num = len(toc_items)
    pages_per_chapter = max(1, total_pages // num) if num > 0 else total_pages

    for i, (title, _href) in enumerate(toc_items):
        page_start = i * pages_per_chapter + 1
        if i + 1 < num:
            page_end = (i + 1) * pages_per_chapter
        else:
            page_end = total_pages
        chapters.append({
            "title": title,
            "order": i,
            "page_start": page_start,
            "page_end": page_end,
        })

    return chapters


def _quality_metrics(text: str) -> tuple[float, int, int]:
    suspicious = 0
    printable = 0
    for char in text:
        if char.isspace():
            continue
        printable += 1
        category = unicodedata.category(char)
        if char == "\ufffd" or category in ("Cc", "Cf", "Co", "Cs"):
            suspicious += 1
    if printable == 0:
        return 0.0, 0, 0
    return suspicious / printable, printable, suspicious


def _repair_char(char: str) -> str:
    if char in ("\n", "\t"):
        return char

    category = unicodedata.category(char)
    if category in ("Cc", "Cf", "Co", "Cs"):
        return ""
    return char
