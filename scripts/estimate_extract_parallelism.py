#!/usr/bin/env python3
"""Estimate staged extract makespan under N batch workers from manifest timings."""

from __future__ import annotations

import heapq
import json
import pathlib
import sys


def estimate_makespan(batch_ms: list[int], workers: int) -> int:
    if not batch_ms:
        return 0
    if workers <= 1:
        return sum(batch_ms)
    heaps = [0] * workers
    heapq.heapify(heaps)
    for duration in sorted(batch_ms, reverse=True):
        current = heapq.heappop(heaps)
        heapq.heappush(heaps, current + duration)
    return max(heaps)


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: estimate_extract_parallelism.py <manifest.json>", file=sys.stderr)
        return 1

    manifest_path = pathlib.Path(sys.argv[1])
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    batches = manifest.get("completed_batches", [])
    batch_ms = [int(batch.get("batch_ms", 0)) for batch in batches if int(batch.get("batch_ms", 0)) > 0]

    summary = {
        "job_id": manifest.get("job_id"),
        "source_path": manifest.get("source_path"),
        "status": manifest.get("status"),
        "page_count": manifest.get("page_count"),
        "batch_size": manifest.get("batch_size"),
        "batch_count": len(batch_ms),
        "sequential_page_extract_ms": manifest.get("page_extract_sum_ms", sum(batch_ms)),
        "wall_page_extract_ms": manifest.get("page_extract_ms", 0),
        "estimates": [],
    }

    for workers in (1, 2, 3, 4):
        makespan = estimate_makespan(batch_ms, workers)
        summary["estimates"].append(
            {
                "workers": workers,
                "estimated_page_extract_ms": makespan,
                "estimated_speedup": round(summary["sequential_page_extract_ms"] / makespan, 3) if makespan else 0,
            }
        )

    print(json.dumps(summary, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
