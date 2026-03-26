#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BOOKS_DIR="${1:-$PROJECT_DIR/books}"
BINARY="$PROJECT_DIR/bin/refloom"
STAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_DIR="$PROJECT_DIR/validation-results/embedding-bench-$STAMP"
BOOK_FILTER="${BENCH_BOOK_FILTER:-}"
MODELS_CSV="${BENCH_EMBEDDING_MODELS:-nomic-embed-text,embeddinggemma}"
BATCH_SIZES_CSV="${BENCH_EMBEDDING_BATCH_SIZES:-16,32,64}"
SKIP_EMBEDDING_SUPPORTED=0

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'make build' first." >&2
  exit 1
fi

if [[ ! -d "$BOOKS_DIR" ]]; then
  echo "ERROR: Books directory $BOOKS_DIR not found." >&2
  exit 1
fi

BOOKS_DIR="$(cd "$BOOKS_DIR" && pwd)"
mkdir -p "$REPORT_DIR/runs"

if "$BINARY" ingest --help 2>&1 | rg -q -- '--skip-embedding'; then
  SKIP_EMBEDDING_SUPPORTED=1
fi

BOOK_CANDIDATES=()
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  BOOK_CANDIDATES+=("$line")
done < <(find "$BOOKS_DIR" -type f \( -name '*.pdf' -o -name '*.epub' \) | sort)
if [[ ${#BOOK_CANDIDATES[@]} -eq 0 ]]; then
  echo "ERROR: No books found in $BOOKS_DIR." >&2
  exit 1
fi

if [[ -n "$BOOK_FILTER" ]]; then
  BOOK_MATCHES=()
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    BOOK_MATCHES+=("$line")
  done < <(printf '%s\n' "${BOOK_CANDIDATES[@]}" | rg --no-messages -- "$BOOK_FILTER" || true)
else
  BOOK_MATCHES=("${BOOK_CANDIDATES[@]}")
fi

if [[ ${#BOOK_MATCHES[@]} -eq 0 ]]; then
  echo "ERROR: No books matched BENCH_BOOK_FILTER=${BOOK_FILTER:-<empty>}." >&2
  exit 1
fi

if [[ ${#BOOK_MATCHES[@]} -ne 1 ]]; then
  printf 'ERROR: BENCH_BOOK_FILTER must select exactly one book, matched %d:\n' "${#BOOK_MATCHES[@]}" >&2
  printf '  %s\n' "${BOOK_MATCHES[@]}" >&2
  exit 1
fi

BOOK_PATH="${BOOK_MATCHES[0]}"
BOOK_NAME="$(basename "$BOOK_PATH")"
GOLDEN_DB="$REPORT_DIR/golden.db"
GOLDEN_PROFILE="$REPORT_DIR/golden-ingest-profile.jsonl"
GOLDEN_LOG="$REPORT_DIR/golden-ingest.log"
GOLDEN_HOME="$REPORT_DIR/home-golden"
REINDEX_PROFILE_SUPPORTED=0

if "$BINARY" reindex --help 2>&1 | rg -q -- '--profile-json'; then
  REINDEX_PROFILE_SUPPORTED=1
fi
if [[ "$SKIP_EMBEDDING_SUPPORTED" -ne 1 ]]; then
  echo "ERROR: current binary does not support 'ingest --skip-embedding' required by benchmark-embedding." >&2
  exit 1
fi
if [[ "$REINDEX_PROFILE_SUPPORTED" -ne 1 ]]; then
  echo "ERROR: current binary does not support 'reindex --profile-json' required by benchmark-embedding." >&2
  exit 1
fi

echo "=== Embedding Benchmark: $STAMP ==="
echo "Book: $BOOK_PATH"
echo "Models: $MODELS_CSV"
echo "Batch sizes: $BATCH_SIZES_CSV"
echo "Report: $REPORT_DIR"
echo ""

mkdir -p "$REPORT_DIR"
start_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"
set +e
golden_ingest_cmd=("$BINARY" ingest --skip-embedding --force --profile-json "$BOOK_PATH")
HOME="$GOLDEN_HOME" \
REFLOOM_DB_PATH="$GOLDEN_DB" \
REFLOOM_EXTRACT_BATCH_WORKERS="${BENCH_GOLDEN_WORKERS:-1}" \
REFLOOM_EMBEDDING_MODEL="${BENCH_GOLDEN_MODEL:-nomic-embed-text}" \
REFLOOM_EMBEDDING_BATCH_SIZE="${BENCH_GOLDEN_BATCH_SIZE:-32}" \
"${golden_ingest_cmd[@]}" >"$GOLDEN_PROFILE" 2>"$GOLDEN_LOG"
golden_exit=$?
set -e
end_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"

if [[ $golden_exit -ne 0 ]]; then
  echo "ERROR: golden ingest failed" >&2
  tail -n 40 "$GOLDEN_LOG" >&2 || true
  exit "$golden_exit"
fi

python3 - "$BOOK_PATH" "$BOOK_NAME" "$GOLDEN_DB" "$GOLDEN_PROFILE" "$GOLDEN_LOG" "$((end_ms - start_ms))" "$SKIP_EMBEDDING_SUPPORTED" "$REPORT_DIR/golden.json" <<'PY'
import json
import pathlib
import sys

book_path, book_name, db_path, profile_path, log_path, wall_ms, skip_supported, summary_path = sys.argv[1:9]
wall_ms = int(wall_ms)
skip_supported = skip_supported == "1"
profile_lines = [line.strip() for line in pathlib.Path(profile_path).read_text(encoding="utf-8").splitlines() if line.strip()]
if not profile_lines:
    raise SystemExit("no golden ingest profile captured")

profile = json.loads(profile_lines[-1])
if profile.get("status") != "completed":
    raise SystemExit(
        "golden ingest did not complete successfully: "
        f"status={profile.get('status')} error={profile.get('error')}"
    )
summary = {
    "book": {
        "path": book_path,
        "name": book_name,
    },
    "db_path": db_path,
    "profile_path": profile_path,
    "log_path": log_path,
    "wall_ms": wall_ms,
    "skip_embedding_supported": skip_supported,
    "embedding_skipped": bool(profile.get("embed_skipped")),
    "ingest_profile": profile,
}
pathlib.Path(summary_path).write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

copy_sqlite_database() {
  local src="$1"
  local dst="$2"

  cp "$src" "$dst"
  for suffix in -wal -shm; do
    if [[ -f "${src}${suffix}" ]]; then
      cp "${src}${suffix}" "${dst}${suffix}"
    fi
  done
}

run_embedding_benchmark() {
  local model="$1"
  local batch_size="$2"
  local run_dir="$REPORT_DIR/runs/${model//\//_}-bs${batch_size}"
  local home_dir="$REPORT_DIR/homes/${model//\//_}-bs${batch_size}"
  local db_path="$run_dir/refloom.db"
  local log_path="$run_dir/run.log"
  local summary_path="$run_dir/run.json"
  local start_ms end_ms exit_code

  mkdir -p "$run_dir" "$home_dir"
  copy_sqlite_database "$GOLDEN_DB" "$db_path"
  start_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"
  set +e
  local -a reindex_cmd=("$BINARY" reindex --embedding --profile-json)
  HOME="$home_dir" \
  REFLOOM_DB_PATH="$db_path" \
  REFLOOM_EMBEDDING_MODEL="$model" \
  REFLOOM_EMBEDDING_BATCH_SIZE="$batch_size" \
  "${reindex_cmd[@]}" >"$run_dir/stdout.txt" 2>"$log_path"
  exit_code=$?
  set -e
  end_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"

  if [[ $exit_code -ne 0 ]]; then
    echo "ERROR: embedding benchmark run failed for model=$model batch_size=$batch_size" >&2
    tail -n 40 "$log_path" >&2 || true
    exit "$exit_code"
  fi

  python3 - "$model" "$batch_size" "$db_path" "$log_path" "$run_dir/stdout.txt" "$((end_ms - start_ms))" "$summary_path" <<'PY'
import json
import pathlib
import re
import sys

model = sys.argv[1]
batch_size = int(sys.argv[2])
db_path = sys.argv[3]
log_path = sys.argv[4]
stdout_path = sys.argv[5]
wall_ms = int(sys.argv[6])
summary_path = pathlib.Path(sys.argv[7])

text = pathlib.Path(log_path).read_text(encoding="utf-8")
stdout_text = pathlib.Path(stdout_path).read_text(encoding="utf-8")
candidate_text = stdout_text + "\n" + text
metrics = {}
stdout_lines = [line.strip() for line in stdout_text.splitlines() if line.strip()]
if stdout_lines and stdout_lines[-1].startswith("{"):
    try:
        metrics.update(json.loads(stdout_lines[-1]))
    except json.JSONDecodeError:
        pass
if not metrics:
    raise SystemExit("missing reindex profile JSON output")

summary = {
    "model": model,
    "batch_size": batch_size,
    "db_path": db_path,
    "log_path": log_path,
    "wall_ms": wall_ms,
    "status": "ok",
    "request_ms": int(metrics.get("request_ms", metrics.get("request_ms", "0")) or 0),
    "save_ms": int(metrics.get("save_ms", metrics.get("save_ms", "0")) or 0),
    "total_ms": int(metrics.get("total_ms", metrics.get("duration", "0")) or 0),
    "chunks": int(metrics.get("chunks", "0") or 0),
    "batches": int(metrics.get("batches", "0") or 0),
    "fails": int(metrics.get("fails", "0") or 0),
    "metrics": metrics,
}
summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY
}

for model in ${MODELS_CSV//,/ }; do
  [[ -z "$model" ]] && continue
  for batch_size in ${BATCH_SIZES_CSV//,/ }; do
    [[ -z "$batch_size" ]] && continue
    run_embedding_benchmark "$model" "$batch_size"
  done
done

python3 - "$REPORT_DIR" <<'PY'
import json
import pathlib
import statistics
import sys

report_dir = pathlib.Path(sys.argv[1])
runs = []
for path in sorted((report_dir / "runs").glob("*/run.json")):
    runs.append(json.loads(path.read_text(encoding="utf-8")))

if not runs:
    raise SystemExit("no embedding benchmark runs captured")

summary = {
    "type": "embedding-benchmark",
    "book": json.loads((report_dir / "golden.json").read_text(encoding="utf-8"))["book"],
    "golden_db": json.loads((report_dir / "golden.json").read_text(encoding="utf-8"))["db_path"],
    "skip_embedding_supported": json.loads((report_dir / "golden.json").read_text(encoding="utf-8"))["skip_embedding_supported"],
    "models": sorted({run["model"] for run in runs}),
    "batch_sizes": sorted({run["batch_size"] for run in runs}),
    "runs": runs,
    "best_wall_ms": min(run.get("wall_ms", 0) for run in runs),
    "median_wall_ms": statistics.median(run.get("wall_ms", 0) for run in runs),
}
(report_dir / "embedding-benchmark.json").write_text(
    json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)
PY

echo "Embedding benchmark complete: $REPORT_DIR/embedding-benchmark.json"
