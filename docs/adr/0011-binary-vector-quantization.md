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

情報理論的バイナリ量子化 (Maximally Informative Binarization, MIB) の研究 [31] では:
- ベクトルを 1 ビット単位に変換 → メモリ消費 1/32
- ハミング距離計算は CPU ビット演算で数倍〜数十倍高速
- インデックスがコンパクトになるため近似 (ANN) ではなく全探索が可能
- 検索精度の低下を完全に排除

## Decision

**2 段階検索: バイナリ量子化で候補絞り込み → float32 で精密スコアリング。**

### 実装方針

1. `chunk_vec_binary` テーブルを追加 (sqlite-vec の bit vector、768 bit = 96 bytes/chunk)
2. ingest 時に float32 embedding からバイナリ版を同時生成
   - 各次元の閾値 (中央値 or 0) でバイナリ化
3. 検索時:
   - Phase 1: バイナリベクトルでハミング距離 top-K*5 を取得 (高速)
   - Phase 2: 候補に対して float32 でコサイン類似度を再計算 (高精度)
4. float32 ベクトルは保持 (精度保証 + re-ranking 用)

### 検討した選択肢

| 選択肢 | 判定 | 理由 |
|--------|------|------|
| バイナリ量子化 + re-rank | **採用** | メモリ 1/32、精度劣化なし、sqlite-vec で実装可能 |
| IVF/HNSW 移行 | 不採用 | sqlite-vec が HNSW 未サポート。別 DB 必要 |
| PQ (Product Quantization) | 不採用 | sqlite-vec でのサポートなし |
| バイナリのみ (re-rank なし) | 不採用 | 精度低下リスクが未検証 |

## Consequences

### Positive

- メモリ消費: 127 MB → ~4 MB (バイナリ部分のみ使う高速検索パス)
- 500 冊超のスケーリングが O(n) 全探索のままで現実的になる
- float32 保持により精度保証を維持

### Negative

- DB サイズは微増 (バイナリテーブル追加分 ~4 MB)
- ingest 時に 2 つのベクトルを保存するオーバーヘッド (軽微)
- sqlite-vec の bit vector サポートの動作検証が必要

### Neutral

- 既存の float32 ベクトルとの互換性は完全に維持
- `reindex` コマンドでバイナリインデックスを再構築可能
