"""Text quality detection for extracted content."""

import unicodedata


def looks_text_corrupt(text: str) -> bool:
    """Check if text appears to be corrupted (mojibake, control chars).

    Samples up to 4000 characters and checks for:
    - Unicode replacement characters (U+FFFD)
    - Control characters (Cc), format characters (Cf),
      private use (Co), surrogates (Cs)

    Returns True if:
    - 3+ consecutive suspicious characters found, OR
    - 2%+ of printable characters are suspicious
    """
    sample = text[:4000]
    if not sample:
        return False

    suspicious = 0
    printable = 0
    suspicious_run = 0
    max_suspicious_run = 0

    for char in sample:
        if char.isspace():
            suspicious_run = 0
            continue
        printable += 1
        category = unicodedata.category(char)
        is_suspicious = char == "\ufffd" or category in ("Cc", "Cf", "Co", "Cs")

        if is_suspicious:
            suspicious += 1
            suspicious_run += 1
            max_suspicious_run = max(max_suspicious_run, suspicious_run)
        else:
            suspicious_run = 0

    if printable == 0:
        return False

    return max_suspicious_run >= 3 or suspicious / printable >= 0.02


def _sample_positions(total: int, n: int = 9) -> list[int]:
    """Pick sample positions from start, middle, and end."""
    if total == 0:
        return []
    if total <= n:
        return list(range(total))

    mid = total // 2
    positions = [
        0, 1, 2,
        max(0, mid - 1), mid, min(total - 1, mid + 1),
        max(0, total - 3), max(0, total - 2), total - 1,
    ]
    # Deduplicate and sort
    return sorted(set(p for p in positions if 0 <= p < total))


def classify_extraction(pages: list[dict]) -> str:
    """Classify extraction quality.

    Returns one of:
    - "ok": text extracted successfully
    - "ocr_required": no text content (likely scanned image PDF)
    - "extract_failed": extraction returned no pages
    - "text_corrupt": mojibake or encoding issues detected
    """
    if not pages:
        return "extract_failed"

    # Check if all pages have empty text
    non_empty = [p for p in pages if p.get("text", "").strip()]
    if not non_empty:
        return "ocr_required"

    # Sample pages for corruption
    positions = _sample_positions(len(non_empty))
    corrupt_count = 0
    for pos in positions:
        text = non_empty[pos].get("text", "")
        if looks_text_corrupt(text):
            corrupt_count += 1

    total_checked = len(positions)
    if total_checked > 0 and (corrupt_count >= 3 or corrupt_count / total_checked >= 0.25):
        return "text_corrupt"

    return "ok"
