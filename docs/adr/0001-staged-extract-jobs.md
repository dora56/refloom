# ADR-0001: Extract を Staged File-Backed Job に変更する

- Status: Accepted
- Date: 2026-03-26
- Deciders: Refloom maintainers

## Context

既存の extract は Python worker へ 1 回の `extract` コマンドを送り、書籍全体の抽出と chunking を一括で返す方式だった。OCR-heavy PDF では処理時間が長くなり、`worker_per_file` timeout に達すると進捗をすべて失って再試行も本単位になっていた。

## Decision

extract を破壊的に再設計し、Go CLI が `probe -> extract-pages -> chunk` を順次 orchestrate する staged job へ変更する。中間成果物は `~/.refloom/work/<job-id>/` に永続化し、manifest・pages JSONL・chunks JSONL を持つ。timeout は file 単位ではなく `worker_probe`, `worker_batch`, `worker_chunk` に分割する。

## Consequences

### Positive

- OCR-heavy PDF でも page batch 単位で再試行でき、途中進捗を失いにくい
- `probe_ms`, `page_extract_ms`, `chunk_ms`, `ocr_ms` を分離計測できる
- `--profile-json` と validation artifact から extract の内訳を追いやすい

### Negative

- worker protocol は互換性がなくなり、旧 `extract` コマンドは廃止される
- `~/.refloom/work` に中間ファイルが残るため cleanup 戦略が必要になる
- 実装が増え、manifest 管理と JSONL マージの失敗経路を考慮する必要がある

### Neutral

- 初期実装では page batch は逐次実行で、並列化は導入しない
- embedding 最適化や DB 書き込み最適化とは独立した改善軸として扱う
