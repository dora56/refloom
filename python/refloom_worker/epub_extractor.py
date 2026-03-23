"""EPUB extraction using ebooklib and BeautifulSoup."""

import ebooklib
from ebooklib import epub
from bs4 import BeautifulSoup


def extract_epub(path: str) -> dict:
    """Extract text and structure from an EPUB file.

    Returns a dict with:
      - book: {title, author, format, page_count}
      - chapters: [{title, order, page_start, page_end}]
      - pages: [{page_num, text}]  (page_num is spine item index)
    """
    book = epub.read_epub(path, options={"ignore_ncx": False})

    # Extract metadata
    raw_title = _get_metadata(book, "title")
    # Some EPUBs have placeholder titles like "(blank)" - treat as empty
    if raw_title and "(blank)" not in raw_title.lower() and raw_title.strip().lower() not in ("unknown", "untitled", ""):
        title = raw_title
    else:
        title = _filename_title(path)
    author = _get_metadata(book, "creator") or ""

    # Extract TOC
    toc_items = _extract_toc(book)

    # Extract text from spine items (reading order)
    spine_items = list(book.get_items_of_type(ebooklib.ITEM_DOCUMENT))
    spine_ids = [item_id for item_id, _ in book.spine]
    ordered_items = []
    id_to_item = {item.get_id(): item for item in spine_items}
    for sid in spine_ids:
        if sid in id_to_item:
            ordered_items.append(id_to_item[sid])

    # If spine ordering fails, fall back to all document items
    if not ordered_items:
        ordered_items = spine_items

    pages = []
    for i, item in enumerate(ordered_items):
        html = item.get_content().decode("utf-8", errors="replace")
        soup = BeautifulSoup(html, "html.parser")
        text = soup.get_text(separator="\n", strip=True)
        if text.strip():
            pages.append({"page_num": i + 1, "text": text})

    # Build chapters from TOC or fallback
    chapters = _build_chapters_from_toc(toc_items, len(pages))
    if not chapters:
        chapters = [{"title": "全体", "order": 0, "page_start": 1, "page_end": len(pages)}]

    return {
        "book": {
            "title": title,
            "author": author,
            "format": "epub",
            "page_count": len(pages),
        },
        "chapters": chapters,
        "pages": pages,
    }


def _get_metadata(book, field: str) -> str:
    values = book.get_metadata("DC", field)
    if values:
        return values[0][0]
    return ""


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
