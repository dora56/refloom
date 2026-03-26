#!/usr/bin/env bash
# Refloom automated validation pipeline.
# Usage: ./scripts/validate_refloom.sh [BOOKS_DIR]
#
# Requires: bin/refloom (run make build first), Ollama running, books in BOOKS_DIR.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BOOKS_DIR="${1:-$PROJECT_DIR/books}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
ARTIFACT_DIR="$PROJECT_DIR/validation-results/$TIMESTAMP"
BINARY="$PROJECT_DIR/bin/refloom"
QUERY_SET="$PROJECT_DIR/testdata/query-set.json"
SCORER="$PROJECT_DIR/scripts/score_validation.py"
BOOK_FILTER="${VALIDATE_BOOK_FILTER:-}"
MAX_BOOKS="${VALIDATE_MAX_BOOKS:-}"
SKIP_INSPECT="${VALIDATE_SKIP_INSPECT:-}"
SKIP_SEARCH="${VALIDATE_SKIP_SEARCH:-}"
SKIP_ASK="${VALIDATE_SKIP_ASK:-}"
SKIP_SCORE="${VALIDATE_SKIP_SCORE:-}"
FRESH_DB="${VALIDATE_FRESH_DB:-}"

# Check prerequisites
if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: $BINARY not found. Run 'make build' first." >&2
  exit 1
fi

if [[ ! -f "$QUERY_SET" ]]; then
  echo "ERROR: $QUERY_SET not found." >&2
  exit 1
fi

if [[ ! -d "$BOOKS_DIR" ]]; then
  echo "ERROR: Books directory $BOOKS_DIR not found." >&2
  exit 1
fi
BOOKS_DIR="$(cd "$BOOKS_DIR" && pwd)"

mkdir -p "$ARTIFACT_DIR"
if [[ -n "$FRESH_DB" && -z "${REFLOOM_DB_PATH:-}" ]]; then
  export REFLOOM_DB_PATH="$ARTIFACT_DIR/validate.db"
fi
echo "=== Refloom Validation: $TIMESTAMP ==="
echo "Artifacts: $ARTIFACT_DIR"
echo ""

artifact_stem_for_book() {
  local book="$1"
  local books_dir="$2"
  local basename rel_path safe_name book_key

  books_dir="${books_dir%/}"
  if [[ -z "$books_dir" ]]; then
    books_dir="/"
  fi

  basename="$(basename "$book")"
  rel_path="${book#"$books_dir"/}"
  if [[ "$rel_path" == "$book" ]]; then
    rel_path="$basename"
  fi

  safe_name="$(printf "%s" "$basename" | tr ' ' '_')"
  book_key="$(printf "%s" "$rel_path" | shasum -a 256 | cut -c1-12)"
  printf "ingest-%s-%s" "$safe_name" "$book_key"
}

# --- Environment ---
{
  echo "timestamp: $TIMESTAMP"
  echo "go: $(go version 2>/dev/null || echo 'n/a')"
  echo "binary: $("$BINARY" version 2>/dev/null || echo 'n/a')"
  echo "ollama: $(curl -sf http://localhost:11434/api/tags | head -c 200 2>/dev/null || echo 'not running')"
  echo "embedding_model_env: ${REFLOOM_EMBEDDING_MODEL:-default}"
  echo "embedding_batch_size_env: ${REFLOOM_EMBEDDING_BATCH_SIZE:-default}"
  echo "extract_batch_workers_env: ${REFLOOM_EXTRACT_BATCH_WORKERS:-default}"
  echo "ocr_fast_scale_env: ${REFLOOM_OCR_FAST_SCALE:-default}"
  echo "ocr_retry_min_chars_env: ${REFLOOM_OCR_RETRY_MIN_CHARS:-default}"
  echo "db_path_env: ${REFLOOM_DB_PATH:-default}"
  echo "book_filter_env: ${BOOK_FILTER:-default}"
  echo "max_books_env: ${MAX_BOOKS:-default}"
  echo "books_dir: $BOOKS_DIR"
} > "$ARTIFACT_DIR/environment.txt"

# --- Step 1: Ingest ---
echo "--- Step 1: Ingest ---"
BOOK_FILES=$(find "$BOOKS_DIR" -type f \( -name '*.pdf' -o -name '*.epub' \) | sort)
if [[ -n "$BOOK_FILTER" ]]; then
  BOOK_FILES=$(printf "%s\n" "$BOOK_FILES" | rg "$BOOK_FILTER" || true)
fi
if [[ -n "$MAX_BOOKS" ]]; then
  BOOK_FILES=$(printf "%s\n" "$BOOK_FILES" | sed -n "1,${MAX_BOOKS}p")
fi
BOOK_COUNT=$(printf "%s\n" "$BOOK_FILES" | sed '/^$/d' | wc -l | tr -d ' ')
echo "Found $BOOK_COUNT books"
if [[ "$BOOK_COUNT" == "0" ]]; then
  echo "ERROR: No books matched the requested filter." >&2
  exit 1
fi

SKIP_INGEST="${SKIP_INGEST:-}"
INGEST_START_MS=$(python3 -c 'import time; print(round(time.time() * 1000))')
if [[ -n "$SKIP_INGEST" ]]; then
  echo "  SKIP_INGEST set — skipping ingest (using existing DB)"
else
  while IFS= read -r book; do
    [[ -z "$book" ]] && continue
    BASENAME="$(basename "$book")"
    echo "  Ingesting: $BASENAME"
    ARTIFACT_STEM="$(artifact_stem_for_book "$book" "$BOOKS_DIR")"
    PROFILE_PATH="$ARTIFACT_DIR/$ARTIFACT_STEM.json"
    LOG_PATH="$ARTIFACT_DIR/$ARTIFACT_STEM.txt"
    "$BINARY" ingest "$book" --force --profile-json \
      > "$PROFILE_PATH" \
      2> >(tee "$LOG_PATH" >&2) || true
  done <<< "$BOOK_FILES"
fi
INGEST_END_MS=$(python3 -c 'import time; print(round(time.time() * 1000))')
INGEST_DURATION_MS=$((INGEST_END_MS - INGEST_START_MS))
echo "  Ingest time: ${INGEST_DURATION_MS}ms"
echo "$INGEST_DURATION_MS" > "$ARTIFACT_DIR/timing-ingest.txt"

python3 - "$ARTIFACT_DIR" <<'PY'
import json
import pathlib
import statistics
import sys

artifact_dir = pathlib.Path(sys.argv[1])
profiles = []
for path in sorted(artifact_dir.glob("ingest-*.json")):
    text = path.read_text(encoding="utf-8").strip()
    if not text:
        continue
    for line in text.splitlines():
        line = line.strip()
        if line:
            profiles.append(json.loads(line))

if not profiles:
    sys.exit(0)

completed = [p for p in profiles if p.get("status") == "completed"]
summary = {
    "books": len(profiles),
    "completed_books": len(completed),
    "effective_books": sum(1 for p in completed if p.get("chunks", 0) > 0),
    "models": sorted({p.get("embed_model", "") for p in profiles if p.get("embed_model")}),
    "embedding_batch_sizes": sorted({p.get("embed_batch_size", 0) for p in completed if p.get("embed_batch_size", 0) > 0}),
    "extract_worker_counts": sorted({p.get("extract_workers_used", 0) for p in completed if p.get("extract_workers_used", 0) > 0}),
    "total_embed_batches": sum(p.get("embed_batches", 0) for p in completed),
    "total_chunks": sum(p.get("chunks", 0) for p in completed),
    "total_ocr_pages": sum(p.get("ocr_pages", 0) for p in completed),
    "total_ocr_retries": sum(p.get("ocr_retries", 0) for p in completed),
    "total_ocr_ms": sum(p.get("ocr_ms", 0) for p in completed),
    "total_probe_ms": sum(p.get("probe_ms", 0) for p in completed),
    "total_page_extract_ms": sum(p.get("page_extract_ms", 0) for p in completed),
    "total_page_extract_sum_ms": sum(p.get("page_extract_sum_ms", 0) for p in completed),
    "total_chunk_ms": sum(p.get("chunk_ms", 0) for p in completed),
    "total_ocr_fast_pages": sum(p.get("ocr_fast_pages", 0) for p in completed),
    "total_ocr_retry_pages": sum(p.get("ocr_retry_pages", 0) for p in completed),
    "total_batch_count": sum(p.get("batch_count", 0) for p in completed),
    "total_failed_batch_count": sum(p.get("failed_batch_count", 0) for p in completed),
    "parallel_extract_books": sum(1 for p in completed if p.get("parallel_extract_enabled")),
    "resumed_books": sum(1 for p in completed if p.get("resumed")),
    "median_total_ms": statistics.median([p.get("total_ms", 0) for p in completed]) if completed else 0,
    "median_extract_ms": statistics.median([p.get("extract_ms", 0) for p in completed]) if completed else 0,
    "median_probe_ms": statistics.median([p.get("probe_ms", 0) for p in completed]) if completed else 0,
    "median_page_extract_ms": statistics.median([p.get("page_extract_ms", 0) for p in completed]) if completed else 0,
    "median_page_extract_sum_ms": statistics.median([p.get("page_extract_sum_ms", 0) for p in completed]) if completed else 0,
    "median_chunk_ms": statistics.median([p.get("chunk_ms", 0) for p in completed]) if completed else 0,
    "median_embed_ms": statistics.median([p.get("embed_ms", 0) for p in completed]) if completed else 0,
    "median_ocr_ms": statistics.median([p.get("ocr_ms", 0) for p in completed]) if completed else 0,
    "profiles": profiles,
}

(artifact_dir / "ingest-summary.json").write_text(
    json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)
PY

if [[ -z "$SKIP_INSPECT" ]]; then
  # --- Step 2: Inspect ---
  echo ""
  echo "--- Step 2: Inspect ---"
  "$BINARY" inspect --stats 2>&1 | tee "$ARTIFACT_DIR/inspect.txt"
fi

QUERY_IDS=""
if [[ -z "$SKIP_SEARCH" || -z "$SKIP_ASK" || -z "$SKIP_SCORE" ]]; then
  QUERY_IDS=$(python3 -c "import json; qs=json.load(open('$QUERY_SET')); [print(q['id']) for q in qs]")
fi

if [[ -z "$SKIP_SEARCH" ]]; then
  # --- Step 3: Search queries ---
  echo ""
  echo "--- Step 3: Search (keyword + hybrid) ---"
  for qid in $QUERY_IDS; do
    QUERY=$(python3 -c "import json; qs=json.load(open('$QUERY_SET')); q=[x for x in qs if x['id']=='$qid'][0]; print(q['query'])")

    echo "  [$qid] keyword: $QUERY"
    "$BINARY" search "$QUERY" --mode fts --limit 5 --json > "$ARTIFACT_DIR/$qid.keyword.json" 2>/dev/null || echo '{"query":"'"$QUERY"'","mode":"fts","count":0,"results":[]}' > "$ARTIFACT_DIR/$qid.keyword.json"

    echo "  [$qid] hybrid:  $QUERY"
    "$BINARY" search "$QUERY" --mode hybrid --limit 5 --json > "$ARTIFACT_DIR/$qid.hybrid.json" 2>/dev/null || echo '{"query":"'"$QUERY"'","mode":"hybrid","count":0,"results":[]}' > "$ARTIFACT_DIR/$qid.hybrid.json"
  done
fi

if [[ -z "$SKIP_ASK" ]]; then
  # --- Step 4: Ask queries (subset) ---
  echo ""
  echo "--- Step 4: Ask ---"
  # Use first 3 queries for ask (LLM calls are expensive)
  ASK_IDS=$(echo "$QUERY_IDS" | head -3)

  for qid in $ASK_IDS; do
    QUERY=$(python3 -c "import json; qs=json.load(open('$QUERY_SET')); q=[x for x in qs if x['id']=='$qid'][0]; print(q['query'])")
    echo "  [$qid] ask: $QUERY"
    "$BINARY" ask "$QUERY" --limit 5 --json > "$ARTIFACT_DIR/$qid.ask.json" 2>/dev/null || echo '{"query":"'"$QUERY"'","answer":"","sources":[],"retrieval_ms":0,"generation_ms":0,"total_ms":0}' > "$ARTIFACT_DIR/$qid.ask.json"
  done
fi

if [[ -z "$SKIP_SCORE" ]]; then
  # --- Step 5: Score ---
  echo ""
  echo "--- Step 5: Score ---"
  python3 "$SCORER" "$QUERY_SET" "$ARTIFACT_DIR" | tee "$ARTIFACT_DIR/score.txt"
fi

echo ""
echo "=== Validation complete: $ARTIFACT_DIR ==="
