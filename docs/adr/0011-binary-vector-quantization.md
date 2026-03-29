# ADR-0011: バイナリベクトル量子化

- Status: Proposed
- Date: 2026-03-28
- Deciders: dora56

## Context

現在の sqlite-vec は 768 次元の float32 ベクトル (3072 bytes/chunk) で KNN 検索を行っている。
ADR-0003 で「500 冊超で IVF/HNSW 移行が必要」と記録したが、根本的な問題はメモリ消費と計算コスト。

v0.2.0 時点のデータ:
- 14 冊、8,656 チャンク
- embedding ストレージ: DB サイズの ~85% (149.8 MB 中 ~127 MB)
- KNN は O(n) の全探索

情報理論的バイナリ量子化 (MIB) の研究では:
- ベクトルを 1 ビット単位に変換 → メモリ消費 1/32
- ハミング距離計算は CPU ビット演算で高速
- インデックスがコンパクトになるため高速な全探索が可能

## Decision

**2 段階検索: バイナリ量子化で候補絞り込み → float32 で精密スコアリング。**

### 実装方針

1. `chunk_vec_binary` テーブルを追加 (sqlite-vec の bit vector、768 bit = 96 bytes/chunk)
2. ingest 時に float32 embedding からバイナリ版を生成
   - sqlite-vec の `vec_quantize_binary()` を SQL 経由で実行（閾値は 0 固定）
   - Go バインディングに bit vector ヘルパーがないため SQL 関数経由が必須
3. 検索時:
   - Phase 1: バイナリベクトルでハミング距離 top-K*5 を取得 (高速)
   - Phase 2: 候補 chunk_id に対して float32 embedding を個別取得し、
     Go 側でコサイン類似度を計算して re-rank（sqlite-vec の MATCH は候補絞込に非対応のため）
4. float32 ベクトルは保持 (精度保証 + re-ranking 用)

### 実装上の制約

- **Go バインディング**: `sqlite-vec-go-bindings v0.1.6` は `SerializeFloat32()` のみ提供。
  bit vector は SQL の `vec_quantize_binary()` と `vec_to_json()` 経由で操作する
- **Phase 2 re-rank**: sqlite-vec の `MATCH` は単一クエリの KNN 専用で、
  `WHERE chunk_id IN (...)` との組み合わせ不可。Go 側で `vec_distance_cosine()` スカラー関数を使うか、
  float32 を Go に読み込んでコサイン計算する
- **閾値**: `vec_quantize_binary()` は `> 0.0` 固定閾値。MIB 推奨の中央値ベースは将来検討

### 検討した選択肢

| 選択肢 | 判定 | 理由 |
|--------|------|------|
| バイナリ量子化 + float32 re-rank | **採用** | 検索パスのメモリ 1/32。re-rank により精度低下を軽減 |
| IVF/HNSW 移行 | 不採用 | sqlite-vec が HNSW 未サポート。別 DB 必要 |
| PQ (Product Quantization) | 不採用 | sqlite-vec でのサポートなし |
| バイナリのみ (re-rank なし) | 不採用 | K*5 oversampling でも recall 90-95% 程度。精度低下リスク |

## Consequences

### Positive

- 検索パスメモリ: バイナリ部分は ~0.8 MB (8,656 chunks × 96 bytes)
- 500 冊超でも O(n) 全探索が現実的なレイテンシに収まる可能性
- float32 保持により re-rank で精度を補完

### Negative

- DB サイズ微増 (バイナリテーブル追加分)。float32 は削除しないため節約にはならない
- Go バインディングの制約により SQL 経由の操作が必要（実装複雑度増）
- binary 候補選定で true top-K を逃すリスクあり（recall ~90-95% at K*5 oversampling）
- `vec_quantize_binary()` の閾値が 0 固定で MIB 最適化が適用できない

### Neutral

- 既存の float32 ベクトルとの互換性は完全に維持
- `reindex` コマンドでバイナリインデックスを再構築可能
- PoC で recall 低下の実測が必要（K*5 の oversampling 率の最適化）
