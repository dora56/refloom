#!/usr/bin/env bash
set -euo pipefail

# Review a PR using code-review-remediation-workflow structure
# Usage: ./scripts/review-pr.sh [PR_NUMBER]
# If no PR number, reviews current branch vs main

PR_NUMBER="${1:-}"
cd "$(git rev-parse --show-toplevel)"

if [ -n "$PR_NUMBER" ]; then
  DIFF=$(gh pr diff "$PR_NUMBER" 2>/dev/null || true)
  PR_TITLE=$(gh pr view "$PR_NUMBER" --json title -q '.title' 2>/dev/null || echo "PR #$PR_NUMBER")
  CHANGED_FILES=$(gh pr diff "$PR_NUMBER" --name-only 2>/dev/null || true)
else
  BASE_BRANCH="main"
  DIFF=$(git diff "$BASE_BRANCH"...HEAD 2>/dev/null || true)
  PR_TITLE="$(git log --oneline "$BASE_BRANCH"..HEAD | head -1)"
  CHANGED_FILES=$(git diff "$BASE_BRANCH"...HEAD --name-only 2>/dev/null || true)
fi

if [ -z "$DIFF" ]; then
  echo "No diff found." >&2
  exit 0
fi

FILE_COUNT=$(echo "$CHANGED_FILES" | wc -l | tr -d ' ')

# Determine active lenses based on changed files
LENSES="code-quality, regression/QA"
echo "$CHANGED_FILES" | grep -qE '(auth|token|secret|key|password)' && LENSES="$LENSES, security"
echo "$CHANGED_FILES" | grep -qE '(search|embedding|db/)' && LENSES="$LENSES, performance"
echo "$CHANGED_FILES" | grep -qE '(internal/(cli|config|db|search|citation|llm)/)' && LENSES="$LENSES, architecture"
echo "$CHANGED_FILES" | grep -qE '(\.md|docs/)' && LENSES="$LENSES, documentation"

cat <<REVIEW_PROMPT
/code-review-remediation-workflow

## Review Target
- Title: $PR_TITLE
- Files changed: $FILE_COUNT
- Active lenses: $LENSES

## Changed Files
$(echo "$CHANGED_FILES" | sed 's/^/- /')

## Diff
\`\`\`diff
$DIFF
\`\`\`

Mode: review+remediation
REVIEW_PROMPT
