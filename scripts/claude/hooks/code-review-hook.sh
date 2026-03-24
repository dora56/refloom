#!/usr/bin/env bash
set -euo pipefail

# Code review hook: run code-review-remediation-workflow via Claude Code
# Triggered on PostToolUse for Write|Edit|MultiEdit on .go/.py files

INPUT=$(cat)
FILE=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.file // empty')

if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then
  exit 0
fi

# Only review Go and Python source files (skip tests, generated, configs)
case "$FILE" in
  *.go|*.py) ;;
  *) exit 0 ;;
esac

# Skip test files from review trigger (they'll be reviewed with their source)
case "$FILE" in
  *_test.go|*test_*.py) exit 0 ;;
esac

# Accumulate changed files for batch review at Stop event
REVIEW_DIR="${CLAUDE_PROJECT_DIR:-.}/.claude/.pending-review"
mkdir -p "$REVIEW_DIR"
echo "$FILE" >> "$REVIEW_DIR/files.txt"
