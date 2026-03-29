#!/usr/bin/env python3
"""Score Refloom validation artifacts.

Usage: python3 score_validation.py <query-set.json> <artifact-dir>

Reads per-query JSON outputs from the artifact directory and computes:
- Top-5 hit rate for keyword and hybrid search
- Ask latency percentiles (median, p95)
"""

import json
import math
import sys
import unicodedata
from pathlib import Path


def normalize_title(title: str) -> str:
    """Normalize a book title for comparison using NFKC."""
    return unicodedata.normalize("NFKC", title).strip().lower()


def title_matches(result_title: str, expected_fragment: str) -> bool:
    """Check if a result title contains the expected fragment."""
    return normalize_title(expected_fragment) in normalize_title(result_title)


def percentile(values: list[float], ratio: float) -> float:
    """Compute a percentile value."""
    if not values:
        return 0.0
    s = sorted(values)
    idx = max(0, math.ceil(len(s) * ratio) - 1)
    return s[idx]


def score_search(queries: list[dict], artifact_dir: Path, mode: str) -> dict:
    """Score search results for a given mode (keyword or hybrid)."""
    hits = 0
    total = 0
    per_query = []

    for q in queries:
        qid = q["id"]
        expected = q["expected_books"]
        result_file = artifact_dir / f"{qid}.{mode}.json"

        if not result_file.exists():
            per_query.append({"id": qid, "hit": False, "reason": "missing"})
            total += 1
            continue

        try:
            data = json.loads(result_file.read_text())
        except (json.JSONDecodeError, OSError):
            per_query.append({"id": qid, "hit": False, "reason": "parse_error"})
            total += 1
            continue

        results = data.get("results", [])
        top5_books = [r.get("book_title", "") for r in results[:5]]

        # Check if ALL expected books appear in top-5
        all_found = all(
            any(title_matches(rb, eb) for rb in top5_books)
            for eb in expected
        )

        hits += int(all_found)
        total += 1
        per_query.append({
            "id": qid,
            "category": q.get("category", ""),
            "hit": all_found,
            "top5_books": list(dict.fromkeys(top5_books)),  # dedupe preserving order
            "expected": expected,
        })

    return {
        "mode": mode,
        "hits": hits,
        "total": total,
        "rate": f"{hits}/{total}" if total > 0 else "0/0",
        "per_query": per_query,
    }


def score_ask(queries: list[dict], artifact_dir: Path) -> dict:
    """Score ask results including source book coverage."""
    latencies = []
    retrieval_latencies = []
    generation_latencies = []
    sources_present = 0
    total = 0
    per_query = []

    for q in queries:
        qid = q["id"]
        result_file = artifact_dir / f"{qid}.ask.json"
        if not result_file.exists():
            continue

        try:
            data = json.loads(result_file.read_text())
        except (json.JSONDecodeError, OSError):
            continue

        total += 1
        if data.get("sources"):
            sources_present += 1

        total_ms = data.get("total_ms", 0)
        retrieval_ms = data.get("retrieval_ms", 0)
        generation_ms = data.get("generation_ms", 0)

        if total_ms > 0:
            latencies.append(float(total_ms))
        if retrieval_ms > 0:
            retrieval_latencies.append(float(retrieval_ms))
        if generation_ms > 0:
            generation_latencies.append(float(generation_ms))

        # Source book coverage
        expected_source = q.get("expected_source_books", q.get("expected_books", []))
        source_books = [s.get("book_title", "") for s in data.get("sources", [])]
        if expected_source:
            found = sum(
                1 for eb in expected_source
                if any(title_matches(sb, eb) for sb in source_books)
            )
            coverage = found / len(expected_source) * 100
        else:
            coverage = 100.0 if not source_books else 0.0

        per_query.append({
            "id": qid,
            "category": q.get("category", ""),
            "source_book_coverage": round(coverage, 1),
            "source_books": list(dict.fromkeys(source_books)),
            "expected_source_books": expected_source,
        })

    coverages = [pq["source_book_coverage"] for pq in per_query]
    avg_coverage = sum(coverages) / len(coverages) if coverages else 0.0

    return {
        "total": total,
        "sources_present": sources_present,
        "source_book_coverage_avg": round(avg_coverage, 1),
        "source_book_coverage_per_query": per_query,
        "median_total_ms": percentile(latencies, 0.5),
        "p95_total_ms": percentile(latencies, 0.95),
        "median_retrieval_ms": percentile(retrieval_latencies, 0.5),
        "median_generation_ms": percentile(generation_latencies, 0.5),
    }


def main():
    if len(sys.argv) < 3:
        print(f"Usage: {sys.argv[0]} <query-set.json> <artifact-dir>", file=sys.stderr)
        sys.exit(1)

    query_file = Path(sys.argv[1])
    artifact_dir = Path(sys.argv[2])

    queries = json.loads(query_file.read_text())

    keyword_score = score_search(queries, artifact_dir, "keyword")
    hybrid_score = score_search(queries, artifact_dir, "hybrid")
    ask_score = score_ask(queries, artifact_dir)

    # Print summary
    print("=" * 50)
    print("Refloom Validation Score")
    print("=" * 50)
    print()
    print(f"Keyword hit rate (top-5): {keyword_score['rate']}")
    print(f"Hybrid  hit rate (top-5): {hybrid_score['rate']}")
    print()

    if ask_score["total"] > 0:
        print(f"Ask queries:      {ask_score['total']}")
        print(f"Sources present:  {ask_score['sources_present']}/{ask_score['total']}")
        print(f"Source coverage:  {ask_score['source_book_coverage_avg']:.1f}%")
        print(f"Median total ms:  {ask_score['median_total_ms']:.0f}")
        print(f"P95 total ms:     {ask_score['p95_total_ms']:.0f}")
        print(f"Median retrieval: {ask_score['median_retrieval_ms']:.0f}ms")
        print(f"Median generation:{ask_score['median_generation_ms']:.0f}ms")

        print()
        print("--- Per-query ask source coverage ---")
        for pq in ask_score.get("source_book_coverage_per_query", []):
            books = ", ".join(pq.get("source_books", []))
            print(f"  {pq['id']}: {pq['source_book_coverage']:.0f}%  [{books}]")
    else:
        print("Ask: no results")

    print()
    print("--- Per-query keyword ---")
    for pq in keyword_score["per_query"]:
        status = "HIT" if pq["hit"] else "MISS"
        print(f"  {pq['id']}: {status}")

    print()
    print("--- Per-query hybrid ---")
    for pq in hybrid_score["per_query"]:
        status = "HIT" if pq["hit"] else "MISS"
        books = ", ".join(pq.get("top5_books", []))
        print(f"  {pq['id']}: {status}  [{books}]")

    # Write machine-readable score
    score_json = {
        "keyword": keyword_score,
        "hybrid": hybrid_score,
        "ask": ask_score,
    }
    score_path = artifact_dir / "score.json"
    score_path.write_text(json.dumps(score_json, ensure_ascii=False, indent=2))
    print(f"\nScore JSON: {score_path}")


if __name__ == "__main__":
    main()
