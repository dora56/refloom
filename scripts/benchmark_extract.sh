#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BOOKS_DIR="${1:-$PROJECT_DIR/books}"
BINARY="$PROJECT_DIR/bin/refloom"
STAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_DIR="$PROJECT_DIR/validation-results/extract-bench-$STAMP"
BOOK_FILTER="${BENCH_BOOK_FILTER:-}"
WORKERS_CSV="${BENCH_EXTRACT_WORKERS:-1,2,4,6,8,auto}"
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
if [[ "$SKIP_EMBEDDING_SUPPORTED" -ne 1 ]]; then
  echo "ERROR: current binary does not support 'ingest --skip-embedding' required by benchmark-extract." >&2
  exit 1
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

echo "=== Extract Benchmark: $STAMP ==="
echo "Book: $BOOK_PATH"
echo "Workers: $WORKERS_CSV"
echo "Report: $REPORT_DIR"
echo ""

run_worker_benchmark() {
  local worker="$1"
  local run_dir="$REPORT_DIR/runs/worker-$worker"
  local home_dir="$REPORT_DIR/homes/worker-$worker"
  local db_path="$run_dir/refloom.db"
  local profile_path="$run_dir/profile.jsonl"
  local log_path="$run_dir/run.log"
  local summary_path="$run_dir/run.json"
  local -a cmd=("$BINARY" ingest --skip-embedding --profile-json --force "$BOOK_PATH")

  mkdir -p "$run_dir" "$home_dir"

  local start_ms end_ms elapsed_ms exit_code
  start_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"
  set +e
  HOME="$home_dir" \
  REFLOOM_DB_PATH="$db_path" \
  REFLOOM_EXTRACT_BATCH_WORKERS="$worker" \
  "${cmd[@]}" >"$profile_path" 2>"$log_path"
  exit_code=$?
  set -e
  end_ms="$(python3 -c 'import time; print(round(time.time() * 1000))')"
  elapsed_ms="$((end_ms - start_ms))"

  if [[ $exit_code -ne 0 ]]; then
    echo "ERROR: extract benchmark run failed for worker=$worker" >&2
    tail -n 40 "$log_path" >&2 || true
    exit "$exit_code"
  fi

  python3 "$SCRIPT_DIR/benchmark_extract_profile.py" \
    "$worker" "$BOOK_PATH" "$BOOK_NAME" "$db_path" "$profile_path" "$log_path" "$elapsed_ms" \
    "$SKIP_EMBEDDING_SUPPORTED" "$summary_path"
}

for worker in ${WORKERS_CSV//,/ }; do
  [[ -z "$worker" ]] && continue
  run_worker_benchmark "$worker"
done

python3 "$SCRIPT_DIR/benchmark_extract_summary.py" "$REPORT_DIR"

echo "Extract benchmark complete: $REPORT_DIR/extract-benchmark.json"
