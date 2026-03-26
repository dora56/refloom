#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BOOKS_DIR="${1:-$PROJECT_DIR/books}"
shift || true

MODELS=("$@")
if [[ ${#MODELS[@]} -eq 0 ]]; then
  MODELS=("nomic-embed-text" "embeddinggemma")
fi

if [[ ! -d "$BOOKS_DIR" ]]; then
  echo "ERROR: Books directory $BOOKS_DIR not found." >&2
  exit 1
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_DIR="$PROJECT_DIR/validation-results/model-bench-$STAMP"
mkdir -p "$REPORT_DIR"

for model in "${MODELS[@]}"; do
  echo "=== Benchmarking $model ==="
  export REFLOOM_EMBEDDING_MODEL="$model"
  export REFLOOM_DB_PATH="$REPORT_DIR/$model.db"

  BEFORE_DIRS=$(find "$PROJECT_DIR/validation-results" -maxdepth 1 -mindepth 1 -type d | sort)
  "$SCRIPT_DIR/validate_refloom.sh" "$BOOKS_DIR"
  AFTER_DIRS=$(find "$PROJECT_DIR/validation-results" -maxdepth 1 -mindepth 1 -type d | sort)
  LATEST_DIR=$(comm -13 <(printf "%s\n" "$BEFORE_DIRS") <(printf "%s\n" "$AFTER_DIRS") | tail -1)
  if [[ -z "$LATEST_DIR" ]]; then
    LATEST_DIR=$(find "$PROJECT_DIR/validation-results" -maxdepth 1 -mindepth 1 -type d | sort | tail -1)
  fi
  echo "$model $LATEST_DIR" >> "$REPORT_DIR/runs.txt"
done

cat > "$REPORT_DIR/recommendation.txt" <<'EOF'
Project default recommendation: nomic-embed-text
Change the default only when a future benchmark shows no quality regression and a clear ingest-time win for another model.
EOF

echo "Benchmark complete. Runs:"
cat "$REPORT_DIR/runs.txt"
