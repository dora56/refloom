# ADR-0013: LazyGraphRAG による軽量グラフ検索

- Status: Proposed
- Date: 2026-03-28
- Deciders: dora56

## Context

現在の検索は FTS (キーワード) + vector (セマンティック) のハイブリッドだが、
書籍間の概念的な「つながり」を捉える能力がない。

例: 「ドメインモデリングでドメインを駆動する」と「関数型ドメインモデリング」は
「値オブジェクト」「境界づけられたコンテキスト」等の概念を共有しているが、
現在の検索では同一チャンク内のキーワード一致に頼るため、
概念横断的な質問 (validate q08, q11) でスコアが低い。

GraphRAG [8,9] は文書からエンティティと関係性を抽出して知識グラフを構築するが、
インデックスコストが通常のベクトル検索の 100-1000 倍 [8]。
個人ツールのローカル環境では現実的でない。

LazyGraphRAG [8,11,12] は大規模な事前要約を排除し、クエリ発生時にオンデマンドで
軽量なデータ構造を構築する。インデックスコストを従来の 0.1% に削減。

## Decision

**LazyGraphRAG アプローチで、ingest 時に軽量エンティティ抽出 + SQLite グラフを構築する。**

### 実装方針

#### Phase 1: エンティティ抽出 (ingest 時)

1. chunker 完了後に LLM (ollama) でチャンクからエンティティを抽出
   - エンティティ: 概念、人名、技術用語 (5-15 個/チャンク)
   - 出力: `{entity: string, type: "concept"|"person"|"technology", chunk_id: int}`
2. SQLite テーブル追加:
   - `entity (entity_id, name, type, normalized_name)`
   - `entity_chunk (entity_id, chunk_id)` — 多対多リンク
3. 同一 `normalized_name` のエンティティを自動マージ

#### Phase 2: グラフ検索 (search 時)

1. クエリからエンティティを抽出 (同じ LLM 呼び出し)
2. `entity_chunk` テーブルで関連チャンクを取得
3. 共通エンティティ数でスコアリング → 既存の RRF にグラフスコアとして統合

#### Phase 3: グローバル検索 (将来)

- エンティティの共起関係から「コミュニティ」を構築
- 「この書籍群の主要テーマは？」に回答

### 検討した選択肢

| 選択肢 | 判定 | 理由 |
|--------|------|------|
| LazyGraphRAG (軽量、オンデマンド) | **採用** | コスト 0.1%、SQLite で完結、段階的に拡張可能 |
| 完全な GraphRAG | 不採用 | Leidenアルゴリズム + LLM 要約のコストが高い |
| E2GraphRAG (SpaCy + LLM) | 不採用 | SpaCy の日本語モデルが追加依存 |
| エンティティ抽出なし (embedding のみ) | 不採用 | 概念横断検索の課題が未解決 |

## Consequences

### Positive

- 書籍横断の概念検索が可能になる (q08, q11 の改善)
- SQLite 内で完結 (外部グラフ DB 不要)
- 段階的実装: Phase 1 だけでもエンティティベース検索が可能
- ingest 時のエンティティ抽出は embedding と並列化可能

### Negative

- ingest 時間増加: LLM によるエンティティ抽出に追加コスト (~1-2 秒/チャンク)
- Ollama の LLM 品質に依存 (エンティティ抽出の精度)
- 日本語の概念正規化 (「値オブジェクト」=「Value Object」) が課題
- DB スキーマ変更 (マイグレーション追加)

### Neutral

- 既存の FTS + vector 検索は変更なし。グラフスコアは RRF の 3 番目のソースとして追加
- `--skip-graph` フラグで ingest 時のエンティティ抽出をスキップ可能
- Phase 1 の PoC で効果を検証してから Phase 2-3 に進む
