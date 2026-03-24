#!/usr/bin/env bash
set -euo pipefail

# Cross-review: run Codex CLI to review staged/unstaged changes
# Called from Claude Code hook after ExitPlanMode

cd "$(git rev-parse --show-toplevel)"

DIFF=$(git diff HEAD 2>/dev/null || true)
if [ -z "$DIFF" ]; then
  echo "No changes to review." >&2
  exit 0
fi

echo "==> Running Codex cross-review on current changes..." >&2

codex --quiet \
  "以下の diff をレビューしてください。バグ、設計上の問題、テスト漏れを指摘してください。問題がなければ LGTM と回答してください。

\`\`\`diff
$DIFF
\`\`\`" 2>&1 || true
