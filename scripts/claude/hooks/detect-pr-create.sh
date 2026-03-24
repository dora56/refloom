#!/usr/bin/env bash
set -euo pipefail

# Claude Code hook: detect gh pr create and suggest review
# Triggered on PostToolUse for Bash

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only trigger on gh pr create
case "$COMMAND" in
  *"gh pr create"*|*"gh pr merge"*) ;;
  *) exit 0 ;;
esac

# Get PR number from output or current branch
PR_NUMBER=$(gh pr view --json number -q '.number' 2>/dev/null || true)

if [ -z "$PR_NUMBER" ]; then
  exit 0
fi

cat >&2 <<MSG
[code-review-remediation-workflow] PR #$PR_NUMBER detected.
Run: ./scripts/review-pr.sh $PR_NUMBER
Or use the code-review-remediation-workflow skill to review the PR diff.
MSG
