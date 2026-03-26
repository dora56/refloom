#!/usr/bin/env python3
from __future__ import annotations

import json
import pathlib
import statistics
import sys


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        raise SystemExit("usage: benchmark_extract_summary.py <report_dir>")

    report_dir = pathlib.Path(argv[1])
    runs = []
    for path in sorted((report_dir / "runs").glob("worker-*/run.json")):
        runs.append(json.loads(path.read_text(encoding="utf-8")))

    if not runs:
        raise SystemExit("no extract benchmark runs captured")

    summary = {
        "type": "extract-benchmark",
        "book": runs[0]["book"],
        "workers": [run["worker"] for run in runs],
        "skip_embedding_supported": any(run["skip_embedding_supported"] for run in runs),
        "runs": runs,
        "best_page_extract_ms": min(run.get("page_extract_ms", 0) for run in runs),
        "best_total_ms": min(run.get("total_ms", 0) for run in runs),
        "median_page_extract_ms": statistics.median(run.get("page_extract_ms", 0) for run in runs),
        "median_total_ms": statistics.median(run.get("total_ms", 0) for run in runs),
    }
    (report_dir / "extract-benchmark.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
