# docs/INDEX.md — ドキュメント一覧

## How (手順・設定) — 常に最新化

| ドキュメント | 用途 | 最終更新 |
|---|---|---|
| [../CLAUDE.md](../CLAUDE.md) | Claude Code 向けプロジェクト指示 | 2026-03-27 |
| [../AGENTS.md](../AGENTS.md) | AI エージェント共通プロジェクト指示 | 2026-03-24 |
| [../ARCHITECTURE.md](../ARCHITECTURE.md) | アーキテクチャ概要 (C4 Level 2) | 2026-03-27 |
| [../README.md](../README.md) | ユーザー向け (インストール・使い方) | 2026-03-24 |

## Why (意思決定) — 永続。Superseded で更新

| ドキュメント | 用途 | ステータス |
|---|---|---|
| [adr/0000-template.md](adr/0000-template.md) | ADR テンプレート | Active |
| [adr/0001-staged-extract-jobs.md](adr/0001-staged-extract-jobs.md) | staged extract job への変更記録 | Accepted |
| [adr/0002-go-python-dual-language.md](adr/0002-go-python-dual-language.md) | Go + Python 2 言語構成の選択 | Accepted |
| [adr/0003-sqlite-storage-and-search.md](adr/0003-sqlite-storage-and-search.md) | SQLite + FTS5 + sqlite-vec の採用 | Accepted |
| [adr/0004-apple-vision-ocr.md](adr/0004-apple-vision-ocr.md) | Apple Vision Framework OCR の採用 | Accepted |
| [adr/0005-ollama-nomic-embed-text.md](adr/0005-ollama-nomic-embed-text.md) | Ollama nomic-embed-text の採用 | Accepted |
| [adr/0006-llm-multi-provider-citation-budget.md](adr/0006-llm-multi-provider-citation-budget.md) | LLM マルチプロバイダと引用バジェット | Accepted |
| [adr/0007-persistent-worker-pool.md](adr/0007-persistent-worker-pool.md) | Persistent worker pool の導入 | Accepted |
| [adr/0008-ocr-policy-and-caching.md](adr/0008-ocr-policy-and-caching.md) | OCR ポリシーとキャッシュ | Accepted |
| [adr/0009-embedding-parallel-batching.md](adr/0009-embedding-parallel-batching.md) | Embedding 並列バッチ | Accepted |
| [adr/0010-adjacent-chunk-context.md](adr/0010-adjacent-chunk-context.md) | 隣接チャンクコンテキスト拡張 | Proposed |
| [adr/0011-binary-vector-quantization.md](adr/0011-binary-vector-quantization.md) | バイナリベクトル量子化 | Proposed |
| [adr/0012-iterative-query-refinement.md](adr/0012-iterative-query-refinement.md) | 反復的クエリ書き換え | Proposed |
| [adr/0013-lazy-graph-rag.md](adr/0013-lazy-graph-rag.md) | LazyGraphRAG 軽量グラフ検索 | Proposed |

## What (仕様・評価) — バージョン付き

| ドキュメント | 用途 | 最終更新 |
|---|---|---|
| [reports/poc-evaluation.md](reports/poc-evaluation.md) | PoC 評価 (#1-#16) | 2026-03-23 |
| [reports/poc-final-report.md](reports/poc-final-report.md) | PoC 最終レポート | 2026-03-23 |
| [reports/staged-extract-design.md](reports/staged-extract-design.md) | staged extract の protocol / workdir 設計 | 2026-03-26 |
| [reports/codex-comparison-report.md](reports/codex-comparison-report.md) | Codex CLI 比較レポート | 2026-03-23 |
| [reports/fts-morphology-research.md](reports/fts-morphology-research.md) | FTS 形態素解析リサーチ | 2026-03-23 |
| [reports/scale-estimation.md](reports/scale-estimation.md) | スケール推定 | 2026-03-23 |
| [reports/baseline-scores.json](reports/baseline-scores.json) | 検証ベースラインスコア | 2026-03-23 |
