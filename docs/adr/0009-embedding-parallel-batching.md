# ADR-0009: Embedding Parallel Batching

- Status: Accepted
- Date: 2026-03-27
- Deciders: dora56

## Context

6 冊合計の embedding 処理が 1,272 秒 (21.2 分) かかっている。
56 batch が完全直列で処理され、各 batch の 95-98% が Ollama HTTP リクエストの I/O 待ち。

Ollama は並列リクエストに対応しているが、Go 側が 1 batch ずつ逐次送信している。

## Decision

embedding batch を **2-4 goroutine で並列送信** する。

- `embed_parallel_workers` 設定を追加 (デフォルト 2)
- `saveChunkEmbeddings` の batch ループを goroutine プールに変更
- DB 保存は結果受信側で逐次実行 (SQLite 同時書き込み不可)
- HTTP クライアントに `MaxIdleConnsPerHost` を設定

### 検討した選択肢

| 選択肢 | 説明 | 判定 |
|---|---|---|
| A) 現状維持 (直列) | 改善なし | 却下 |
| **B) goroutine プールで並列** | Ollama の並列処理能力を活用 | **採用** |
| C) streaming embedding | Ollama API が非対応 | 不可 |

### 採用理由

- Ollama は並列 /api/embed リクエストを処理可能
- extract の `extractBatchesConcurrent` と同様のパターンで実装コスト低
- DB 保存を逐次にすることで SQLite の排他ロック制約を回避

## Consequences

### Positive

- 2 並列で ~50% 削減 (1,272s → ~636s)
- 4 並列で ~75% 削減 (1,272s → ~318s)
- reindex コマンドにも同じ改善が適用される

### Negative

- Ollama の GPU メモリ消費が並列度に応じて増加
- エラーハンドリングが複雑化 (並列 batch の部分失敗)

### Neutral

- DB 保存は逐次のまま (SQLite WAL モードでも書き込み排他)
