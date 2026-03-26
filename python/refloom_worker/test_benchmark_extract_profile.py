from __future__ import annotations

import importlib.util
import pathlib
import sys

import pytest

MODULE_PATH = pathlib.Path(__file__).resolve().parents[2] / "scripts" / "benchmark_extract_profile.py"
SPEC = importlib.util.spec_from_file_location("benchmark_extract_profile", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


def test_extract_benchmark_accepts_ocr_required_skip() -> None:
    profile = {
        "status": "skipped",
        "quality": "ocr_required",
        "page_extract_ms": 1200,
        "page_extract_sum_ms": 1150,
        "ocr_ms": 900,
        "embed_skipped": True,
        "extract_auto_max_workers": 8,
        "extract_auto_effective_cap": 6,
        "extract_auto_tier": "pro",
        "extract_auto_candidates": [1, 2, 4, 6],
        "auto_worker_reason": (
            "tier=pro perf_cores=6 free_mem_gb=1.8 avg_batch_ms=4080 "
            "configured_cap=8 effective_cap=6 selected=4"
        ),
    }

    summary = MODULE.build_extract_benchmark_summary(
        worker="2",
        book_path="/tmp/book.pdf",
        book_name="book.pdf",
        db_path="/tmp/refloom.db",
        profile=profile,
        profile_path="/tmp/profile.jsonl",
        log_path="/tmp/run.log",
        elapsed_ms=1300,
        skip_embedding_supported=True,
    )

    assert summary["status"] == "skipped"
    assert summary["benchmark_status"] == "accepted"
    assert summary["benchmark_reason"] == "ocr_required_extract_only"
    assert summary["page_extract_ms"] == 1200
    assert summary["extract_auto_max_workers"] == 8
    assert summary["extract_auto_effective_cap"] == 6
    assert summary["extract_auto_tier"] == "pro"
    assert summary["extract_auto_candidates"] == [1, 2, 4, 6]
    assert summary["auto_worker_reason"] == (
        "tier=pro perf_cores=6 free_mem_gb=1.8 avg_batch_ms=4080 "
        "configured_cap=8 effective_cap=6 selected=4"
    )


def test_extract_benchmark_rejects_unexpected_skipped_status() -> None:
    profile = {
        "status": "skipped",
        "quality": "too_short",
        "embed_skipped": True,
    }

    with pytest.raises(SystemExit, match="did not complete successfully"):
        MODULE.build_extract_benchmark_summary(
            worker="1",
            book_path="/tmp/book.pdf",
            book_name="book.pdf",
            db_path="/tmp/refloom.db",
            profile=profile,
            profile_path="/tmp/profile.jsonl",
            log_path="/tmp/run.log",
            elapsed_ms=100,
            skip_embedding_supported=True,
        )


def test_extract_benchmark_accepts_completed_ingest() -> None:
    profile = {
        "status": "completed",
        "quality": "ok",
        "chunks": 42,
    }

    summary = MODULE.build_extract_benchmark_summary(
        worker="auto",
        book_path="/tmp/book.epub",
        book_name="book.epub",
        db_path="/tmp/refloom.db",
        profile=profile,
        profile_path="/tmp/profile.jsonl",
        log_path="/tmp/run.log",
        elapsed_ms=200,
        skip_embedding_supported=True,
    )

    assert summary["status"] == "completed"
    assert summary["benchmark_status"] == "accepted"
    assert summary["benchmark_reason"] == "completed"
