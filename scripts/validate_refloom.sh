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

mkdir -p "$ARTIFACT_DIR"
echo "=== Refloom Validation: $TIMESTAMP ==="
echo "Artifacts: $ARTIFACT_DIR"
echo ""

# --- Environment ---
{
  echo "timestamp: $TIMESTAMP"
  echo "go: $(go version 2>/dev/null || echo 'n/a')"
  echo "binary: $("$BINARY" version 2>/dev/null || echo 'n/a')"
  echo "ollama: $(curl -sf http://localhost:11434/api/tags | head -c 200 2>/dev/null || echo 'not running')"
  echo "books_dir: $BOOKS_DIR"
} > "$ARTIFACT_DIR/environment.txt"

# --- Step 1: Ingest ---
echo "--- Step 1: Ingest ---"
BOOK_FILES=$(find "$BOOKS_DIR" -type f \( -name '*.pdf' -o -name '*.epub' \) | sort)
BOOK_COUNT=$(echo "$BOOK_FILES" | wc -l | tr -d ' ')
echo "Found $BOOK_COUNT books"

SKIP_INGEST="${SKIP_INGEST:-}"
INGEST_START=$(date +%s)
if [[ -n "$SKIP_INGEST" ]]; then
  echo "  SKIP_INGEST set — skipping ingest (using existing DB)"
else
  for book in $BOOK_FILES; do
    BASENAME="$(basename "$book")"
    echo "  Ingesting: $BASENAME"
    "$BINARY" ingest "$book" --force 2>&1 | tee "$ARTIFACT_DIR/ingest-$(echo "$BASENAME" | tr ' ' '_').txt" || true
  done
fi
INGEST_END=$(date +%s)
echo "  Ingest time: $((INGEST_END - INGEST_START))s"
echo "$((INGEST_END - INGEST_START))" > "$ARTIFACT_DIR/timing-ingest.txt"

# --- Step 2: Inspect ---
echo ""
echo "--- Step 2: Inspect ---"
"$BINARY" inspect --stats 2>&1 | tee "$ARTIFACT_DIR/inspect.txt"

# --- Step 3: Search queries ---
echo ""
echo "--- Step 3: Search (keyword + hybrid) ---"
QUERY_IDS=$(python3 -c "import json; qs=json.load(open('$QUERY_SET')); [print(q['id']) for q in qs]")

for qid in $QUERY_IDS; do
  QUERY=$(python3 -c "import json; qs=json.load(open('$QUERY_SET')); q=[x for x in qs if x['id']=='$qid'][0]; print(q['query'])")

  echo "  [$qid] keyword: $QUERY"
  "$BINARY" search "$QUERY" --mode fts --limit 5 --json > "$ARTIFACT_DIR/$qid.keyword.json" 2>/dev/null || echo '{"query":"'"$QUERY"'","mode":"fts","count":0,"results":[]}' > "$ARTIFACT_DIR/$qid.keyword.json"

  echo "  [$qid] hybrid:  $QUERY"
  "$BINARY" search "$QUERY" --mode hybrid --limit 5 --json > "$ARTIFACT_DIR/$qid.hybrid.json" 2>/dev/null || echo '{"query":"'"$QUERY"'","mode":"hybrid","count":0,"results":[]}' > "$ARTIFACT_DIR/$qid.hybrid.json"
done

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

# --- Step 5: Score ---
echo ""
echo "--- Step 5: Score ---"
python3 "$SCORER" "$QUERY_SET" "$ARTIFACT_DIR" | tee "$ARTIFACT_DIR/score.txt"

echo ""
echo "=== Validation complete: $ARTIFACT_DIR ==="
