#!/usr/bin/env bash
set -euo pipefail

# Run code-review-remediation-workflow on accumulated changed files
# Triggered at Stop event (when Claude finishes responding)

REVIEW_DIR="${CLAUDE_PROJECT_DIR:-.}/.claude/.pending-review"
FILES_LIST="$REVIEW_DIR/files.txt"

if [ ! -f "$FILES_LIST" ]; then
  exit 0
fi

# Deduplicate and filter existing files
FILES=$(sort -u "$FILES_LIST" | while read -r f; do [ -f "$f" ] && echo "$f"; done || true)
rm -rf "$REVIEW_DIR"

if [ -z "$FILES" ]; then
  exit 0
fi

FILE_COUNT=$(echo "$FILES" | wc -l | tr -d ' ')

# Only trigger review when 3+ source files changed (avoid noise on small edits)
if [ "$FILE_COUNT" -lt 3 ]; then
  exit 0
fi

DIFF=$(git diff HEAD -- $FILES 2>/dev/null || true)
if [ -z "$DIFF" ]; then
  exit 0
fi

# Output review request as stderr (shown to Claude as hook feedback)
cat >&2 <<REVIEW_REQUEST
[code-review-remediation-workflow] $FILE_COUNT files changed. Run /code-review-remediation-workflow on:
$(echo "$FILES" | sed 's/^/  - /')

To start review: use the code-review-remediation-workflow skill with review+remediation mode on the current diff.
REVIEW_REQUEST
