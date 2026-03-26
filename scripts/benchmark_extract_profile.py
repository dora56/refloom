#!/usr/bin/env python3
from __future__ import annotations

import json
import pathlib
import sys
from typing import Any


def build_extract_benchmark_summary(
    *,
    worker: str,
    book_path: str,
    book_name: str,
    db_path: str,
    profile: dict[str, Any],
    profile_path: str,
    log_path: str,
    elapsed_ms: int,
    skip_embedding_supported: bool,
) -> dict[str, Any]:
    status = profile.get("status")
    quality = profile.get("quality")

    accepted = status == "completed"
    benchmark_reason = "completed"
    if not accepted and status == "skipped" and quality == "ocr_required":
        accepted = True
        benchmark_reason = "ocr_required_extract_only"

    if not accepted:
        raise SystemExit(
            f"ingest benchmark run for worker={worker} did not complete successfully: "
            f"status={status} quality={quality} error={profile.get('error')}"
        )

    summary = {
        "book": {
            "path": book_path,
            "name": book_name,
        },
        "worker": worker,
        "db_path": db_path,
        "profile_path": profile_path,
        "log_path": log_path,
        "wall_ms": elapsed_ms,
        "skip_embedding_supported": skip_embedding_supported,
        "embedding_included": not skip_embedding_supported,
        "ingest_profile": profile,
        "benchmark_status": "accepted",
        "benchmark_reason": benchmark_reason,
    }
    summary.update(
        {
            "status": status,
            "quality": quality,
            "page_extract_ms": profile.get("page_extract_ms", 0),
            "page_extract_sum_ms": profile.get("page_extract_sum_ms", 0),
            "probe_ms": profile.get("probe_ms", 0),
            "chunk_ms": profile.get("chunk_ms", 0),
            "extract_ms": profile.get("extract_ms", 0),
            "embed_ms": profile.get("embed_ms", 0),
            "total_ms": profile.get("total_ms", 0),
            "extract_workers_used": profile.get("extract_workers_used", 0),
            "extract_auto_max_workers": profile.get("extract_auto_max_workers", 0),
            "extract_auto_effective_cap": profile.get("extract_auto_effective_cap", 0),
            "extract_auto_tier": profile.get("extract_auto_tier"),
            "extract_auto_candidates": profile.get("extract_auto_candidates", []),
            "parallel_extract_enabled": profile.get("parallel_extract_enabled", False),
            "auto_worker_reason": profile.get("auto_worker_reason"),
            "batch_count": profile.get("batch_count", 0),
            "failed_batch_count": profile.get("failed_batch_count", 0),
            "chunks": profile.get("chunks", 0),
            "ocr_pages": profile.get("ocr_pages", 0),
            "ocr_ms": profile.get("ocr_ms", 0),
            "embed_batches": profile.get("embed_batches", 0),
        }
    )
    return summary


def main(argv: list[str]) -> int:
    if len(argv) != 10:
        raise SystemExit(
            "usage: benchmark_extract_profile.py "
            "<worker> <book_path> <book_name> <db_path> <profile_path> <log_path> "
            "<elapsed_ms> <skip_embedding_supported> <summary_path>"
        )

    worker = argv[1]
    book_path = argv[2]
    book_name = argv[3]
    db_path = argv[4]
    profile_path = pathlib.Path(argv[5])
    log_path = argv[6]
    elapsed_ms = int(argv[7])
    skip_embedding_supported = argv[8] == "1"
    summary_path = pathlib.Path(argv[9])

    lines = [line.strip() for line in profile_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    if not lines:
        raise SystemExit(f"no ingest profile captured for worker={worker}")

    profile = json.loads(lines[-1])
    summary = build_extract_benchmark_summary(
        worker=worker,
        book_path=book_path,
        book_name=book_name,
        db_path=db_path,
        profile=profile,
        profile_path=str(profile_path),
        log_path=log_path,
        elapsed_ms=elapsed_ms,
        skip_embedding_supported=skip_embedding_supported,
    )
    summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
