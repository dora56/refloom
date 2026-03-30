# ADR-0010: 隣接チャンクコンテキスト拡張

- Status: Accepted
- Date: 2026-03-28
- Accepted: 2026-03-30
- Deciders: dora56

## Context

現在の検索パイプラインは、ヒットしたチャンク単体をそのまま LLM に渡している。
しかし、チャンキング時に文脈が分断されるため、チャンク単体では回答に必要な前提情報が欠けることがある。

例: 「値オブジェクトの不変条件とは」→ ヒットしたチャンクに定義が含まれるが、
直前のチャンクにある具体例がないと LLM が十分な回答を生成できない。

DB には既に `prev_chunk_id` / `next_chunk_id` のリンクが格納されている（chunk テーブル）。
このインフラは Phase C-D で実装済みだが、検索・引用構築では未活用。

## Decision

**検索ヒットの前後 1 チャンクを自動的にコンテキストに含める（同一チャプター内のみ）。**

### 実装方針

1. `enrichResults()` でチャンクを取得した後、`prev_chunk_id` / `next_chunk_id` を使って
   前後のチャンクを DB から取得する
   - **同一チャプター制約**: `chapter_id` が異なる場合は展開しない
   - **重複排除**: 連続ヒット時、同じチャンクが複数回取得される場合は `chunk_id` で除去
2. `search.Result` に `PrevChunk` / `NextChunk` フィールドを追加（`*db.Chunk`、nullable）
3. citation 構築時に `[前チャンク.Body] + [ヒットチャンク.Body] + [後チャンク.Body]` を結合
4. **budget 調整**: 展開有効時は `PerChunk` デフォルトを 500→1200 に変更（3 チャンク分）。
   `PromptBudget` (3000) はそのまま維持し、結果数で自然に調整される
   （展開により 1 結果あたりの消費が増え、含まれる結果数が減る = depth vs breadth のトレードオフ）
5. `--expand-context` フラグで有効化（デフォルト off で PoC 期間中は opt-in）

### 検討した選択肢

| 選択肢 | 判定 | 理由 |
|--------|------|------|
| 前後 1 チャンク自動展開（同一チャプター内） | **採用** | 最小コストで文脈改善。既存インフラ活用 |
| 前後 2 チャンク展開 | 不採用 | budget 消費が大きく、ノイズ混入リスク |
| LLM 判断で動的展開 | 不採用 | 追加 LLM 呼び出しのレイテンシが大きい |
| チャンクサイズ増大 | 不採用 | FTS 精度低下、embedding 品質低下のリスク |
| 短チャンクのみ展開（閾値ベース） | 不採用 | 閾値の決定が難しく、効果が限定的 |

## Consequences

### Positive

- 検索結果の文脈が豊富になり、LLM の回答品質が向上
- 追加の DB クエリのみで実装可能（外部依存なし）
- 既存の `prev_chunk_id` / `next_chunk_id` インフラを活用

### Negative

- 1 結果あたりのコンテキスト量が最大 3 倍 → budget 内の結果数が減少（depth vs breadth）
- DB クエリが最大 3N 回に増加（N 結果 × 3 チャンク）。SQLite PK lookup のため影響は軽微

### Neutral

- `make validate` のスコアへの影響を PoC で計測する必要がある
- PoC 期間中は `--expand-context` で opt-in、効果確認後にデフォルト有効化を検討

## PoC 結果 (2026-03-30)

- validate スコア: keyword 9/12, hybrid 9/12 (リグレッションなし)
- q01 (値オブジェクトの不変条件): expand-context で `KilogramQuantity` の具体例を取得。baseline の `UnitQuantity` とは異なる文脈を提供
- レイテンシ: retrieval 293ms (baseline 1262ms より改善、キャッシュ効果)、total は LLM 生成がボトルネックで同等 (~15s)
- `--expand-context` フラグとして実装完了。デフォルト off で opt-in
