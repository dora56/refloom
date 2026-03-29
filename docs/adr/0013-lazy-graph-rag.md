# ADR-0013: 軽量エンティティグラフ検索

- Status: Proposed
- Date: 2026-03-28
- Deciders: dora56

## Context

現在の検索は FTS (キーワード) + vector (セマンティック) のハイブリッドだが、
書籍間の概念的な「つながり」を捉える能力がない。

例: 「データモデリングでドメインを駆動する」と「関数型ドメインモデリング」は
「値オブジェクト」「境界づけられたコンテキスト」等の概念を共有しているが、
現在の検索では同一チャンク内のキーワード一致に頼るため、
概念横断的な質問 (validate q08, q11) でスコアが低い。

GraphRAG は知識グラフで概念の接続性を捉えるが、インデックスコストが高い。
本 ADR では GraphRAG の概念を参考にしつつ、ローカル環境に適した軽量な
エンティティ抽出 + SQLite グラフを構築する。

**注記**: Microsoft の LazyGraphRAG はクエリ時にオンデマンドでグラフを構築する手法だが、
本 ADR は ingest 時にエンティティを抽出する方式であり、厳密には LazyGraphRAG とは異なる。
コストは通常の GraphRAG よりは大幅に低いが、LazyGraphRAG 論文の 0.1% という数値は
本設計には直接適用できない。

## Decision

**ingest 時に LLM でエンティティを抽出し、SQLite にエンティティグラフを構築する。**

### Phase 1: エンティティ抽出 (ingest 時)

1. chunker 完了後に Ollama (generation) でチャンクからエンティティを抽出
   - エンティティ: 概念、人名、技術用語 (5-15 個/チャンク)
   - 出力: `{entity: string, type: "concept"|"person"|"technology", chunk_id: int}`
2. SQLite テーブル追加 (マイグレーション 003):
   - `entity (entity_id, name, type, normalized_name)`
   - `entity_chunk (entity_id, chunk_id, book_id)` — 多対多リンク + book_id で横断検索効率化
3. 正規化戦略:
   - Unicode NFKC 正規化 + 小文字化
   - 日英対応テーブル (手動管理): 「値オブジェクト」↔「value object」等の主要概念
   - 同一 `normalized_name` のエンティティを自動マージ

### Phase 2: エンティティベース検索 (search 時)

1. クエリからエンティティを抽出
   - **高速パス**: `entity` テーブルの `name` に対する LIKE/FTS マッチ（LLM 不要）
   - **高精度パス**: Ollama generation でクエリからエンティティ抽出（2-5 秒追加）
   - デフォルトは高速パス。`--deep-graph` で高精度パスを使用
2. `entity_chunk` テーブルで関連チャンクを取得
3. 共通エンティティ数でスコアリング → RRF に 3 番目のソースとして統合
   - `reciprocalRankFusion` を可変長入力に拡張 (`[][]db.SearchResult`)

### Phase 3: グローバル検索 (将来)

- エンティティの共起関係から概念クラスタを構築
- 「この書籍群の主要テーマは？」に回答

### ingest 時間の見積り

Ollama generation (7B-8B モデル) での構造化エンティティ抽出:
- 1 チャンクあたり: 3-8 秒 (楽観 1-2 秒は非現実的)
- 500 チャンクの書籍: 25-65 分追加
- embedding と同じ Ollama インスタンスを共有するため並列化は困難

**対策**: `--skip-graph` フラグで opt-in。初回 ingest 後に `refloom graph-build` で後追い構築も検討。

### 検討した選択肢

| 選択肢 | 判定 | 理由 |
|--------|------|------|
| ingest 時エンティティ抽出 + SQLite グラフ | **採用** | SQLite 内で完結、段階的に拡張可能 |
| LazyGraphRAG (query-time 構築) | 不採用 | クエリ時の LLM 呼び出しコストが検索レイテンシに直結 |
| 完全な GraphRAG (Leiden + 要約) | 不採用 | LLM コストが高い。ローカル環境では非現実的 |
| SpaCy NER (E2GraphRAG 方式) | 不採用 | SpaCy 日本語モデルの追加依存。技術用語の認識精度が不十分 |

## Consequences

### Positive

- 書籍横断の概念検索が可能になる (q08, q11 の改善)
- SQLite 内で完結 (外部グラフ DB 不要)
- 段階的実装: Phase 1 だけでもエンティティ一覧・統計が取得可能
- 検索時の高速パス (LIKE マッチ) は LLM 不要で追加レイテンシ最小

### Negative

- ingest 時間大幅増加 (500 チャンクで 25-65 分)。`--skip-graph` で回避可能
- Ollama LLM (7B-8B) のエンティティ抽出品質に依存。ノイズ混入リスク
- 日本語/英語の概念正規化は完全自動化が困難。手動マッピングの保守コスト
- DB スキーマ変更 (マイグレーション 003)

### Neutral

- 既存の FTS + vector 検索は変更なし。グラフスコアは RRF の 3 番目のソースとして追加
- Phase 1 の PoC でエンティティ品質と検索効果を検証してから Phase 2-3 に進む
- エンティティ品質のフィルタリング (最小出現回数、型バリデーション) は PoC で要検討
