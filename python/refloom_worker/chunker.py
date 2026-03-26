"""Text chunking with paragraph awareness and chapter boundary respect."""

import json
from pathlib import Path


def chunk_pages(pages: list, chapters: list, chunk_size: int = 500, chunk_overlap: int = 100) -> list:
    """Chunk extracted pages into smaller pieces, respecting chapter boundaries.

    Args:
        pages: [{page_num, text}]
        chapters: [{title, order, page_start, page_end}]
        chunk_size: Target chunk size in characters
        chunk_overlap: Overlap between consecutive chunks in characters

    Returns:
        List of chunk dicts: [{chapter_order, heading, body, char_count, page_start, page_end, chunk_order}]
    """
    all_chunks = []

    for chapter in chapters:
        chapter_pages = [
            p for p in pages
            if chapter["page_start"] <= p["page_num"] <= chapter["page_end"]
        ]
        if not chapter_pages:
            continue

        # Combine all text from chapter pages
        chapter_text = "\n\n".join(p["text"] for p in chapter_pages)
        page_nums = [p["page_num"] for p in chapter_pages]

        # Split into paragraphs
        paragraphs = _split_paragraphs(chapter_text)

        # Build chunks from paragraphs
        chunks = _assemble_chunks(
            paragraphs, chunk_size, chunk_overlap,
            chapter["title"], chapter["order"],
            page_nums[0] if page_nums else None,
            page_nums[-1] if page_nums else None,
        )
        all_chunks.extend(chunks)

    return all_chunks


def load_pages_jsonl(path: str) -> list:
    pages = []
    with Path(path).open(encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            pages.append(json.loads(line))
    return pages


def write_chunks_jsonl(chunks: list, path: str) -> int:
    output_path = Path(path)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with output_path.open("w", encoding="utf-8") as fh:
        for chunk in chunks:
            fh.write(json.dumps(chunk, ensure_ascii=False) + "\n")
    return len(chunks)


def _split_paragraphs(text: str) -> list:
    """Split text into paragraphs by double newline or multiple newlines."""
    import re
    # Split on two or more newlines
    parts = re.split(r"\n{2,}", text)
    # Filter empty
    return [p.strip() for p in parts if p.strip()]


def _assemble_chunks(
    paragraphs: list,
    chunk_size: int,
    chunk_overlap: int,
    heading: str,
    chapter_order: int,
    page_start: int | None,
    page_end: int | None,
) -> list:
    """Assemble paragraphs into chunks of approximately chunk_size characters."""
    if not paragraphs:
        return []

    chunks = []
    current_parts = []
    current_len = 0
    chunk_order = 0
    overlap_text = ""

    for para in paragraphs:
        # If a single paragraph exceeds 2x chunk_size, force-split it
        if len(para) > chunk_size * 2:
            sub_parts = _force_split(para, chunk_size)
        else:
            sub_parts = [para]

        for part in sub_parts:
            # If adding this part would exceed the target, finalize current chunk
            if current_parts and current_len + len(part) > chunk_size:
                body = "\n\n".join(current_parts)
                if overlap_text:
                    body = overlap_text + "\n\n" + body

                chunks.append({
                    "chapter_order": chapter_order,
                    "heading": heading,
                    "body": body,
                    "char_count": len(body),
                    "page_start": page_start,
                    "page_end": page_end,
                    "chunk_order": chunk_order,
                })
                chunk_order += 1

                # Keep overlap from the end of current body
                overlap_text = body[-chunk_overlap:] if len(body) > chunk_overlap else body
                current_parts = []
                current_len = 0

            current_parts.append(part)
            current_len += len(part)

    # Final chunk
    if current_parts:
        body = "\n\n".join(current_parts)
        if overlap_text and chunk_order > 0:
            body = overlap_text + "\n\n" + body

        chunks.append({
            "chapter_order": chapter_order,
            "heading": heading,
            "body": body,
            "char_count": len(body),
            "page_start": page_start,
            "page_end": page_end,
            "chunk_order": chunk_order,
        })

    return chunks


def _force_split(text: str, chunk_size: int) -> list:
    """Force-split a long text at sentence boundaries (Japanese period or newline)."""
    import re
    # Try to split at Japanese period, question mark, exclamation, or newline
    sentences = re.split(r"(?<=[。？！\n])", text)
    parts = []
    current = ""
    for sent in sentences:
        if len(current) + len(sent) > chunk_size and current:
            parts.append(current.strip())
            current = sent
        else:
            current += sent
    if current.strip():
        parts.append(current.strip())
    return parts if parts else [text]
