# ADR-0003: SQLite 単一 DB によるストレージ・検索基盤

- Status: Accepted
- Date: 2026-03-27
- Deciders: Refloom maintainers

## Context

ローカルファーストの個人読書支援ツールとして、ゼロコンフィグで動作する永続化基盤が必要だった。検索はキーワード検索（日本語対応）とセマンティック検索の両立が求められる。PostgreSQL + pgvector、外部ベクトル DB (Qdrant, Chroma)、SQLite + 拡張の 3 案を検討した。

また、日本語キーワード検索では FTS5 trigram トークナイザ単体で keyword hit rate 0/12 という深刻な問題が判明した。2 文字キーワード（例: 「文章」）が trigram では検索できず、長い自然文クエリが AND 条件化して 0 件になるためである。

## Decision

SQLite を mattn/go-sqlite3 (CGO) 経由で唯一の DB として採用する。検索基盤として以下を組み合わせる。

- **FTS5 dual table**: `chunk_fts` (trigram トークナイザ) + `chunk_fts_seg` (kagome 形態素解析 + unicode61)
- **sqlite-vec**: 768 次元 float ベクトルによるブルートフォース KNN
- **Reciprocal Rank Fusion (k=60)**: FTS BM25 スコアとコサイン類似度を統合
- **比較 intent 検出 + book 多様化**: 比較クエリ時にラウンドロビンで複数書籍を返す

スキーマ管理は `schema_version` テーブルによるマイグレーションで行う。

## Consequences

### Positive

- 外部サービス不要、単一ファイルバックアップで完結する
- kagome 導入で keyword hit rate 0/12 → 9/12 に改善した
- FTS5 + sqlite-vec + RRF が単一プロセス内で動作し、レイテンシが低い

### Negative

- CGO 必須のためクロスコンパイルが制約される
- sqlite-vec のブルートフォース KNN は O(n) であり、~500 冊で ANN インデックス (IVF/HNSW) への移行が必要になる
- kagome (IPA 辞書) がバイナリサイズに ~50MB 加算される

### Neutral

- reranker は現在の ~100 冊規模では不要と判断し、導入を保留している
- スケール推定の詳細は [docs/reports/scale-estimation.md](../reports/scale-estimation.md) を参照
- FTS 形態素解析の検証は [docs/reports/fts-morphology-research.md](../reports/fts-morphology-research.md) を参照
