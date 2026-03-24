"""PDF extraction using PyMuPDF (fitz) with optional Vision Framework OCR."""

import sys

import fitz


def extract_pdf(path: str) -> dict:
    """Extract text, TOC, and page info from a PDF file.

    Returns a dict with:
      - book: {title, author, format, page_count}
      - chapters: [{title, order, page_start, page_end}]
      - pages: [{page_num, text}]
    """
    doc = fitz.open(path)

    # Extract metadata
    metadata = doc.metadata or {}
    title = metadata.get("title", "") or _filename_title(path)
    author = metadata.get("author", "")

    # Extract TOC for chapter structure
    toc = doc.get_toc(simple=True)  # [[level, title, page_num], ...]
    chapters = _build_chapters(toc, len(doc))

    # If no TOC found, treat entire document as one chapter
    if not chapters:
        chapters = [{"title": "全体", "order": 0, "page_start": 1, "page_end": len(doc)}]

    # Extract text page by page, with OCR fallback for image pages
    ocr_available = _check_ocr_available()
    pages = []
    for i in range(len(doc)):
        page = doc[i]
        text = page.get_text("text")

        # If page has no text and OCR is available, try Vision OCR
        if not text.strip() and ocr_available:
            import sys as _sys
            print(f"  OCR page {i + 1}/{len(doc)}...", file=_sys.stderr, flush=True)
            text = _ocr_page(page)

        pages.append({"page_num": i + 1, "text": text})

    doc.close()

    return {
        "book": {
            "title": title,
            "author": author,
            "format": "pdf",
            "page_count": len(pages),
        },
        "chapters": chapters,
        "pages": pages,
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


def _check_ocr_available() -> bool:
    """Check if Vision Framework OCR is available."""
    if sys.platform != "darwin":
        return False
    try:
        from refloom_worker.ocr_vision import is_available
        return is_available()
    except ImportError:
        return False


def _ocr_page(page) -> str:
    """OCR a single PDF page using Vision Framework.

    Renders the page to a high-resolution PNG, then passes to Vision OCR.
    """
    try:
        from refloom_worker.ocr_vision import recognize_text

        # Render at 2x zoom (balance between OCR accuracy and speed)
        pix = page.get_pixmap(matrix=fitz.Matrix(2, 2), alpha=False)
        image_data = pix.tobytes(output="png")

        texts = recognize_text(image_data)
        return "\n".join(texts)
    except Exception:
        return ""
