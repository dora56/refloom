# CLAUDE.md — Refloom

@AGENTS.md

## Claude Code 固有の指示

- 実験や検証は git worktree を使って行うこと
- Python 依存管理には pip ではなく uv を使うこと
- コミットメッセージは英語、動詞始まり、簡潔に
- 設計判断を伴う変更には ADR (docs/adr/) を作成すること
- `make ci` が通ることを確認してからコミットすること
