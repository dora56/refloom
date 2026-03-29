# PoC 実行計画: ADR-0010〜0013 検索改善

## Context

v0.2.0 リリース後、RAG 技術動向レポートに基づき 4 件の ADR (0010-0013) を Proposed で作成済み。
validate スコア keyword 9/12, hybrid 9/12 の改善（特に q08/q11 cross-book 質問）が目標。
ADR レビュー完了済み。PoC を worktree で実施し、効果を検証する。

Codex CLI レビュー (2026-03-28) による Critical 指摘を反映:
- C-1: 現行 validate は ask 回答品質を採点しておらず、PoC-2 の改善が検証不能 → PoC-0 で評価基盤を補強
- C-2: 0012 のレイテンシ見積り不整合 → HyDE 優先 + timeout 拡張検討
- C-3: 0013 は ingest 常設が重すぎる → offline build コマンドに限定

---

## PoC 実行順序と判断基準

```
PoC-0: 評価基盤補強 (1 日)
  → validate_refloom.sh の ask 対象拡張 + source_book_coverage 指標追加
  → PoC-1 以降の Go/No-Go 判定に必須

PoC-1: ADR-0010 隣接チャンクコンテキスト拡張 (0.5-1.5 日)
  → ask の回答品質改善を新評価基盤で計測
  → 効果あり → Accepted, main にマージ

PoC-2: ADR-0012 query-expansion PoC — HyDE vs 反復書き換え比較 (3-5 日)
  → HyDE を先に実装（実装・計測がシンプル）
  → 反復書き換えと A/B 比較
  → q08/q11 の改善を計測。効果の高い方を Accepted

PoC-3: ADR-0011 バイナリベクトル量子化 (3-5 日)
  → recall@5/10 と median/p95 retrieval_ms を計測
  → 現スケール (14冊) では優先度低。蔵書増加時に再評価

PoC-4: ADR-0013 軽量エンティティグラフ Phase 1 (5-10 日)
  → offline `graph-build` コマンドとして実装（ingest パスに入れない）
  → PoC-1〜3 の結果を踏まえて着手判断
```

**各 PoC の Go/No-Go 基準**: validate スコアが現状以上を維持しつつ、対象指標が改善すること。
改善なし or リグレッションの場合は ADR を Deprecated に変更。

---

## PoC-0: 評価基盤補強 (1 日)

### 目的
ask の回答品質を定量的に評価できるようにする。現行 validate は検索 hit rate と
ask のソース有無・レイテンシしか見ておらず、回答品質を採点していない。
**validate_refloom.sh は ask を q01-q03 の 3 件のみ実行しており、q08/q11 の評価が不可能。**

### 変更対象ファイル

| ファイル | 変更内容 |
|---------|---------|
| `scripts/validate_refloom.sh` | ask 対象を q01-q03 → cross-book (q08, q11) を含む 5 件に拡張 |
| `scripts/score_validation.py` | ask スコアに `source_book_coverage` 指標を追加 |
| `testdata/query-set.json` | 各クエリに `expected_source_books` フィールドを追加 |

### 追加する評価指標

1. **source_book_coverage**: ask の回答に含まれる引用ソースの expected_books カバー率
   - 各 ask クエリの `--json` 出力から引用ソースの book title を抽出
   - `expected_source_books` との一致率を計算 (0-100%)
   - cross-book クエリ (q08, q11) で特に重要

2. **answer_relevance_score** (手動評価テンプレート):
   - PoC 実施時に人手で 3 段階評価 (good / partial / poor)
   - `validation-results/<run>/answer_relevance.json` に JSON で記録
   - schema: `[{query_id, score: "good"|"partial"|"poor", notes: ""}]`

### 成功基準

- validate_refloom.sh が q08/q11 を含む ask を実行
- score_validation.py が `source_book_coverage` (per-query + 平均) を出力
- cross-book クエリ (q08, q11) の baseline coverage を記録

---

## PoC-1: ADR-0010 隣接チャンクコンテキスト拡張

### 目的
検索ヒットの前後 1 チャンクをコンテキストに含め、ask の回答品質を改善する。

### 変更対象ファイル

| ファイル | 変更内容 |
|---------|---------|
| `internal/search/search.go` | `Result` struct に `PrevChunk`/`NextChunk` 追加。`enrichResults()` で隣接チャンク取得 |
| `internal/citation/citation.go` | `PromptOptions` に `ExpandContext bool` 追加。`BuildPromptWithBudget()` で 3 チャンク結合 |
| `internal/cli/ask.go` | `--expand-context` フラグ追加 |
| `internal/search/search_test.go` | 隣接チャンク取得: 同一チャプター制約、重複排除テスト |
| `internal/citation/citation_test.go` | ExpandContext=true での budget 制御テスト |

### 実装ステップ

**Step 1: search.Result 拡張**
```go
type Result struct {
    ChunkID   int64
    Score     float64
    PrevChunk *db.Chunk  // 新規: 同一チャプター内の前チャンク
    Chunk     *db.Chunk
    NextChunk *db.Chunk  // 新規: 同一チャプター内の後チャンク
    Chapter   *db.Chapter
    Book      *db.Book
}
```

**Step 2: enrichResults() に隣接チャンク取得を追加**
- `chunk.PrevChunkID.Valid` の場合、`GetChunkByID(chunk.PrevChunkID.Int64)` で取得
- 同一 `ChapterID` チェック: `prevChunk.ChapterID != chunk.ChapterID` なら nil
- NextChunk も同様
- 重複排除: `seen` map[int64]bool で同じ chunk_id の二重取得を防止

**Step 3: citation.BuildPromptWithBudget() 拡張**
- `PromptOptions.ExpandContext` が true の場合:
  - body を `[prev.Body]\n\n[chunk.Body]\n\n[next.Body]` に結合
  - `PerChunk` を 1200 に設定（3 チャンク分）
- false の場合は既存動作を維持
- **注意**: `ask.go` は `cfg.PromptChunkLimit` を明示的に `PromptOptions.PerChunk` に渡している。
  `--expand-context` 有効時は `ask.go` 側で `PerChunk` を 1200 に上書きする必要がある（DefaultPromptOptions() の変更だけでは無効）

**Step 4: テスト (TDD)**
- `citation_test.go`: ExpandContext=true での budget 制御、PerChunk=1200 での切り詰め
- `search_test.go`: 同一チャプター制約、重複排除

### 検証方法

```bash
# worktree で実施
git worktree add ../refloom-poc-0010 -b poc/adr-0010

# テスト
make ci

# A/B 比較 (PoC-0 の評価基盤を使用)
make validate                                                            # baseline
./bin/refloom ask --expand-context "値オブジェクトの不変条件とは" --json  # expanded
# source_book_coverage と人手 3 段階評価で比較
```

### 成功基準

- `make ci` 全パス
- `make validate` スコア: keyword ≥9/12, hybrid ≥9/12 (リグレッションなし)
- source_book_coverage が baseline 以上
- 人手評価で 3 問中 2 問以上が good or partial → good に改善

---

## PoC-2: ADR-0012 query-expansion PoC — HyDE vs 反復書き換え

### 目的
q08/q11 の cross-book 検索精度を改善する。2 つのアプローチを比較:
- **A: HyDE (Hypothetical Document Embeddings)** — 先に実装（シンプル）
- **B: 反復的クエリ書き換え** — A の結果を見て実装判断

### HyDE 実装 (優先)

| ファイル | 変更内容 |
|---------|---------|
| `internal/cli/ask.go` | `--hyde` フラグ。HyDE orchestration を CLI 層に配置 |

**レイヤ設計**: 現行 `search.Engine` は DB + embedding client のみ保持し、LLM を持たない。
HyDE は LLM 呼び出し (仮回答生成) を必要とするため、`search.Engine` に `SearchHyDE()` を
追加するのではなく、**CLI 層 (`ask.go`) で HyDE orchestration** を行う。

```
1. ask.go: LLM に「この質問に対する仮の回答」を生成させる (200 chars 以内)
2. ask.go: 仮回答テキストを embedClient.Embed() で embedding 化
3. ask.go: engine.Search() を 2 回呼び出し:
   - 通常のハイブリッド検索 (query)
   - ベクトル検索のみ (HyDE embedding)
4. ask.go: 2 つの結果を chunk_id で重複排除してマージ
5. ask.go: マージ結果を citation → LLM 回答
```

HyDE は LLM 1 回 + embedding 1 回で完結し、レイテンシが予測しやすい。
`llm.Provider` の文字列出力をそのまま使えるため、JSON パースの問題もない。
`search.Engine` の責務は変更しない。

**工数見積り**: 3-5 日 (LLM orchestration + merge ロジック + テスト + 計測)

### 反復書き換え実装 (A の結果次第)

| ファイル | 変更内容 |
|---------|---------|
| `internal/cli/ask.go` | `--max-refine N` フラグ。書き換えループ |

HyDE が q08/q11 を改善しない場合のみ着手。
タイムアウト: `ask` timeout を 120s に拡張（`--max-refine` 有効時のみ）。

### 検証方法

```bash
# q08, q11 それぞれで A/B 比較
./bin/refloom ask "ドメインモデリングと関数型プログラミングの共通点" --json             # baseline
./bin/refloom ask "ドメインモデリングと関数型プログラミングの共通点" --hyde --json      # A: HyDE

# source_book_coverage で定量比較
# 人手 3 段階評価で定性比較
```

### 成功基準

- q08 または q11 で source_book_coverage が baseline より改善
- p95 total_ms ≤ 60s (HyDE) / ≤ 120s (反復書き換え)
- `make validate` リグレッションなし

---

## PoC-3: ADR-0011 バイナリベクトル量子化

### 目的
vector 検索のメモリ効率化と高速化。500 冊スケールの現実性を検証。

### 変更対象ファイル

| ファイル | 変更内容 |
|---------|---------|
| `internal/db/migrations/003_binary_vec.sql` | `chunk_vec_binary` テーブル追加 |
| `internal/db/vector.go` | バイナリ量子化 (SQL 経由) + 2 段階検索 |
| `internal/cli/reindex.go` | `--binary` フラグでバイナリインデックス再構築 |

### 検証方法

```bash
# バイナリインデックス構築
./bin/refloom reindex --binary

# 計測 (PoC-0 は不要、検索品質の定量測定)
# recall@5: float32 top-5 vs binary-rerank top-5 の chunk_id 一致率
# recall@10: 同上 top-10
# median retrieval_ms: 検索レイテンシ中央値
# p95 retrieval_ms: 検索レイテンシ 95 パーセンタイル
```

### 成功基準

- recall@10 ≥ 90% (float32 との一致)
- median retrieval_ms が改善 or 同等
- `make validate` リグレッションなし

**注記**: 現スケール (14冊, 8,656 chunks) ではレイテンシ改善が体感困難な可能性あり。
蔵書 50 冊超で再評価するのが現実的。

---

## PoC-4: ADR-0013 軽量エンティティグラフ Phase 1

### 目的
Ollama でのエンティティ抽出品質と、エンティティベース検索の効果を検証。
**ingest パスには入れず、offline `graph-build` コマンドとして実装。**

### 変更対象ファイル

| ファイル | 変更内容 |
|---------|---------|
| `internal/db/migrations/003_entity.sql` | entity, entity_chunk テーブル |
| `internal/cli/graph.go` | `refloom graph-build` コマンド (新規) |
| `internal/search/search.go` | RRF 3 ソース対応 (`reciprocalRankFusion` 可変長化) |

### 検証方法 (Phase 1 のみ)

```bash
# offline でエンティティ抽出 (ingest とは独立)
./bin/refloom graph-build                  # 全書籍
./bin/refloom graph-build --book-id 41     # 1 冊だけ

# エンティティ品質評価 (50-100 チャンクのサンプル)
./bin/refloom graph-stats                  # entity 数、型別内訳、正規化マージ数

# 手動評価: precision (有意味な概念の割合)、重複率、正規化ミス率
```

### 成功基準

- エンティティ precision ≥ 80% (50 チャンクサンプルで手動評価)
- エンティティ重複率 ≤ 20% (同一概念の非マージ重複)
- 日英同一概念の正規化マージ率 ≥ 50%
- 正規化ミス率 ≤ 10% (誤マージ)
- 1 冊の graph-build 時間 ≤ 30 分 (500 チャンク)
- PoC-1〜3 で q08/q11 が未改善の場合のみ検索統合に進む

### 前提: ADR-0013 の修正

計画は offline `graph-build` コマンドに限定するが、ADR-0013 の Decision は
「ingest 時に LLM でエンティティ抽出」のまま。PoC 着手前に ADR-0013 を修正し、
Decision を「offline graph-build コマンド」に変更する。

---

## worktree 戦略

```
main (v0.2.0)
├── poc/eval-baseline (worktree: ../refloom-poc-eval)  ← PoC-0
├── poc/adr-0010 (worktree: ../refloom-poc-0010)       ← PoC-1 (0 完了後)
├── poc/adr-0012 (worktree: ../refloom-poc-0012)       ← PoC-2 (1 完了後)
├── poc/adr-0011 (worktree: ../refloom-poc-0011)       ← PoC-3 (独立)
└── poc/adr-0013 (worktree: ../refloom-poc-0013)       ← PoC-4 (1-3 完了後に判断)
```

PoC-0 → PoC-1 → PoC-2 は連続。
PoC-3 は独立して並行可能。
PoC-4 は PoC-1〜3 の結果を見て着手判断。

**Migration 番号注意**: PoC-3 と PoC-4 は両方 `003_*` migration を想定。
先にマージされた方が `003`、後が `004` を使用。worktree では仮番号で実装し、
main マージ時に最終番号を確定する。

---

## Codex レビュー指摘への対応

### 初回レビュー (2026-03-28)

| 指摘 | Severity | 対応 |
|------|----------|------|
| 現行 validate は ask 回答品質を採点していない | Critical | PoC-0 で validate_refloom.sh 改修 + source_book_coverage + 人手 3 段階評価を追加 |
| 0012 レイテンシ不整合 | Critical | HyDE 優先 (LLM 1 回で完結)。反復書き換えは timeout 120s に拡張 |
| 0013 は ingest 常設が重すぎる | Critical | offline `graph-build` コマンドに限定 |
| 0010 成功基準が弱い | Warning | source_book_coverage + 人手評価を追加 |
| 0011 工数過小 | Warning | 1-2 日 → 3-5 日に修正 |
| 0011 成功基準が DB サイズ | Warning | recall@5/10 + retrieval_ms に変更 |
| 0012 工数過小 | Warning | 1 日 → 3-5 日に修正 |
| 0012 成功基準が粗い | Warning | source_book_coverage + p95 total_ms を追加 |
| 0012 HyDE を先に比較すべき | Warning | HyDE 優先に変更。query-expansion PoC として切り直し |
| 0013 entity 品質基準がない | Warning | precision + 重複率 + 正規化ミス率を閾値付きで追加 |
| 0013 ingest ではなく offline | Info | `graph-build` コマンドに限定 |

### 再レビュー (2026-03-30)

| 指摘 | Severity | 対応 |
|------|----------|------|
| validate_refloom.sh が ask を q01-q03 のみ実行。PoC-0 変更対象に未含 | Critical | PoC-0 変更対象に `validate_refloom.sh` を追加。ask 対象を q08/q11 含む 5 件に拡張 |
| PoC-1 PerChunk=1200 が ask 経路に効かない | Warning | ask.go で --expand-context 時に PerChunk を明示上書きする設計を追記 |
| PoC-2 HyDE の SearchHyDE() が Engine の責務と不整合 | Warning | HyDE orchestration を CLI 層 (ask.go) に配置する設計に変更。Engine 責務は変更しない |
| ADR-0013 Decision が ingest 時抽出のまま | Warning | PoC-4 着手前に ADR-0013 修正を前提条件に追記 |
| PoC-4 entity 品質の重複率・正規化ミス率に閾値なし | Warning | 重複率 ≤20%、正規化ミス率 ≤10% の閾値を追加 |
| PoC-0 工数 0.5 日は厳しい | Warning | 1 日に修正 |
| PoC-3/4 migration 番号衝突 | Info | worktree 戦略に番号管理ルールを追記 |
