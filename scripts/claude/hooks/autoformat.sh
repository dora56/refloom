#!/usr/bin/env bash
set -euo pipefail

# Auto-format files after Write/Edit/MultiEdit
# Reads PostToolUse JSON from stdin to get the file path

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.file // empty')

if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then
  exit 0
fi

case "$FILE" in
  *.go)
    gofmt -w "$FILE" 2>/dev/null || true
    ;;
  *.py)
    if command -v ruff &>/dev/null; then
      ruff format "$FILE" 2>/dev/null || true
      ruff check --fix "$FILE" 2>/dev/null || true
    elif command -v uv &>/dev/null; then
      uv run --group dev ruff format "$FILE" 2>/dev/null || true
      uv run --group dev ruff check --fix "$FILE" 2>/dev/null || true
    fi
    ;;
esac
