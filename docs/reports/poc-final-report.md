# Refloom PoC 完了レポート

## 概要

Refloom はローカルファーストの CLI 読書支援ツールである。PDF/EPUB の個人蔵書に対して、テキスト抽出・インデックス・検索・LLM による質問応答を提供する。

本 PoC は「コンパクトなローカルスタックで実用的な読書支援が可能か」を検証し、**全 7 成功条件を達成して PoC を完了**した。あわせて 2026-03-26 時点で staged extract 導入後の fresh DB validate を完了し、次フェーズの性能改善 baseline を確定した。

---

## 実装サマリ

### 基本実装 (Phase 0〜7)

| Phase | 内容 |
|---|---|
| 0 | プロジェクトスキャフォールド (Go + cobra + Makefile) |
| 1 | Python worker (PDF/EPUB 抽出 + チャンキング) |
| 2 | SQLite DB (FTS5 trigram + sqlite-vec 768次元) |
| 3 | Ingest パイプライン (抽出 → DB → Embedding) |
| 4 | Hybrid 検索 (FTS + Vector + RRF fusion) |
| 5 | 回答生成 (Claude API + 出典制御) |
| 6 | inspect / reindex コマンド |
| 7 | 設定ファイル (YAML + 環境変数) |

### 運用改善 (#1〜#16)

| # | 内容 | 効果 |
|---|---|---|
| 1 | 構造化ログ (slog) | デバッグ容易性向上 |
| 2 | Embedding リトライ (指数バックオフ) | 一時障害耐性 |
| 3 | タイムアウト設定化 | 大規模 PDF 対応 |
| 4 | Python worker 堅牢化 | エラー伝播改善 |
| 5 | DB マイグレーション管理 | スキーマ進化対応 |
| 6 | ファイルハッシュ重複防止 | 同一コンテンツ検出 |
| 7 | バージョン情報 + 配布パッケージ | `make dist` で ZIP 配布 |
| 8 | EPUB テキストクリーニング | レイアウト artifacts 除去 |
| 9 | 自動検証パイプライン | `make validate` で再現可能な評価 |
| 10 | 比較意図検出 + 書籍分散 | cross-book 検索改善 |
| 11 | 抽出品質分類 | ok/ocr_required/text_corrupt の自動判定 |
| 12 | チャンクリンク (prev/next) | 章内ナビゲーション基盤 |
| 13 | プロンプトバジェット制御 | LLM 入力サイズの明示管理 |
| 14 | FTS 形態素解析 (kagome) | 日本語 keyword 検索 0→9/12 |
| 15 | キーワードエイリアス + スケール見積もり | 同義語展開 + 100冊見積もり |
| 16 | Apple Vision OCR | 画像PDF 対応 (6/6冊有効化) |
| 17 | staged extract jobs | OCR-heavy PDF の全損回避、resume、段階計測 |

---

## 最終アーキテクチャ

```
┌───────────────────────────────────────────┐
│  refloom CLI  (Go + cobra)                │
├─────────┬─────────┬───────────┬───────────┤
│ ingest  │ search  │  ask      │ inspect   │
│         │         │           │ reindex   │
│         │         │           │ version   │
├─────────┴─────────┴───────────┴───────────┤
│ internal packages                          │
│  search/  (hybrid, intent, diversify,     │
│            alias)                           │
│  db/      (SQLite, FTS5, vec, kagome)     │
│  citation/ (prompt budget)                 │
│  embedding/ (Ollama + retry)               │
│  extraction/ (Python worker IPC)           │
│  llm/     (Claude CLI / API)              │
│  config/  (YAML + env)                    │
├────────────────────────────────────────────┤
│ Go staged extract orchestration            │
│  probe -> extract-pages -> chunk          │
│  workdir: manifest / pages / chunks /     │
│           metrics in ~/.refloom/work      │
├────────────────────────────────────────────┤
│ Python Worker (subprocess + JSON)          │
│  pdf_extractor (PyMuPDF + Vision OCR)     │
│  epub_extractor (ebooklib + clean_text)   │
│  chunker (pages JSONL -> chunks JSONL)    │
│  quality (text corruption detection)      │
│  ocr_vision (Apple Vision Framework)      │
├────────────────────────────────────────────┤
│ SQLite (mattn/go-sqlite3)                  │
│  book, chapter, chunk (with prev/next)    │
│  chunk_fts (trigram) + chunk_fts_seg      │
│  chunk_vec (sqlite-vec, float[768])       │
│  ingest_log, schema_version               │
├────────────────────────────────────────────┤
│ Ollama (nomic-embed-text default, embeddinggemma benchmark) │ Claude CLI │
└──────────────────────────┴─────────────────┘
```

---

## 最終検証結果

検証日: 2026-03-26 | コーパス: 6冊 (PDF 3 + EPUB 3)

| 指標 | 結果 |
|---|---|
| 有効書籍 | **6/6** (画像PDF含む) |
| 総チャンク数 | 3,377 |
| Keyword hit rate (top-5) | **9/12** |
| Hybrid hit rate (top-5) | **10/12** |
| Ask ソース付き回答 | **3/3** |
| Ask median total latency | **14.8 秒** |
| Ask median retrieval latency | **97 ms** |
| Ask median generation latency | **14.7 秒** |

### 2026-03-26 fresh DB staged baseline

| 指標 | 値 |
|---|---:|
| 6冊 total ingest | **2,299,764 ms** |
| median total ms | **181,404.5** |
| median extract ms | **2,577.0** |
| median embed ms | **164,043.0** |
| total page extract ms | **1,142,634** |
| total ocr ms | **1,090,210** |

- baseline artifact:
  - `validation-results/20260326-075122`
- OCR-heavy PDF (`455 pages / 1272 chunks`) は `page_extract_ms=1112133`, `ocr_ms=1087558`, `batch_count=29` で完走し、旧設計で出ていた `signal: killed` は再発しなかった。

### OCR-heavy extract benchmark (`1,2,4,6,8,auto`)

| ケース | page_extract_ms | page_extract_sum_ms | total_ms | extract_workers_used |
|---|---:|---:|---:|---:|
| fixed `1` | 119,956 | 116,489 | 120,657 | 1 |
| fixed `2` | 66,310 | 127,113 | 66,636 | 2 |
| fixed `4` | 45,569 | 171,442 | 45,888 | 4 |
| fixed `6` | **38,542** | 215,226 | **38,857** | 6 |
| fixed `8` | 38,622 | 278,159 | 38,930 | 8 |
| `auto` | 75,245 | 136,161 | 75,535 | 2 |

- benchmark artifact:
  - `validation-results/extract-bench-20260326-165204`
- `page_extract_ms` は wall clock、`page_extract_sum_ms` は batch 実行時間の合計を表す。
- Apple Silicon 向け auto heuristic は `extract_auto_max_workers=8`、`perf_cores=10`, `free_mem_gb=1.8`, `avg_batch_ms=4093` を観測し、`reason=tier=max ... selected=2` で `workers=2` を選んだ。
- 同じ本の fixed 比較では `workers=6` と `workers=8` が最速帯で、`workers=10` を含む旧 sweep では `workers=8` が最速・`10` で悪化した。したがって、この PC では OCR-heavy PDF の fixed 推奨帯は `6..8` である。

### Text-heavy extract guardrail

- benchmark artifact:
  - `validation-results/extract-bench-20260326-170019`
- text-heavy PDF (`関数型ドメインモデリング`) では fixed `2/4/6/8` と `auto` のすべてで `extract_workers_used=1` を維持した。
- これにより、`workers>=2` は OCR-heavy PDF にのみ適用し、EPUB / text-heavy PDF は逐次のままにする制約が benchmark で確認できた。

### 2026-03-25 embedding モデル比較 baseline

| モデル | 6冊 total ingest | median total ms | median embed ms | keyword | hybrid |
|---|---:|---:|---:|---:|---:|
| `nomic-embed-text` | **2,242,058 ms** | **217,747.5** | **215,636.5** | 9/12 | 10/12 |
| `embeddinggemma` | 3,403,530 ms | 405,064.5 | 402,821.5 | 9/12 | 10/12 |

- baseline artifact:
  - `validation-results/20260325-132505` (`nomic-embed-text`)
  - `validation-results/20260325-140336` (`embeddinggemma`)
- この比較により、**既定の embedding モデルは `nomic-embed-text`** とする。

### 改善の推移

| 指標 | Phase 7 時点 | #8 時点 | 最終 (#16) |
|---|---|---|---|
| 有効書籍 | 5/6 | 5/6 | **6/6** |
| Keyword hit rate | 0/12 | 0/12 | **9/12** |
| Hybrid hit rate | 未計測 | 9/12 | **10/12** |

---

## Codex 実装との比較

同一仕様に対して Codex CLI が独立実装した PoC と比較評価を行った (`docs/codex-comparison-report.md`)。

### 取り込んだ知見

| 知見 | 対応 |
|---|---|
| 自動検証パイプライン | #9 で導入 |
| 比較意図検出 + 書籍分散 | #10 で導入 |
| 抽出品質の4段階分類 | #11 で導入 |
| チャンクリンク (prev/next) | #12 で導入 |
| プロンプトバジェット制御 | #13 で導入 |
| キーワードエイリアス | #15 で導入 |

### 本実装の優位点

- パッケージ分離による保守性
- ネイティブ DB アクセス (CGO + sqlite-vec)
- 運用堅牢性 (#1〜#7)
- staged extract による OCR-heavy PDF の全損回避
- テストコード (最新 CI で Python 47件 + Go)

---

## PoC 成功条件の達成状況

| # | 条件 | 判定 |
|---|---|---|
| 1 | EPUB/PDF から実用レベルで抽出できる | **PASS** (6/6冊、OCR含む) |
| 2 | 3〜5冊に対して検索が有効に機能する | **PASS** (hybrid 10/12) |
| 3 | `ask` で要約中心の有用な回答が返る | **PASS** (3/3) |
| 4 | 回答に本名・章名・ページ範囲を含められる | **PASS** |
| 5 | 長文引用を抑えられる | **PASS** |
| 6 | M2 MacBook Pro 上で実用的に動作する | **PASS** (OCR-heavy PDF が 20分超でも完走) |
| 7 | 本体 + Python ワーカー構成の妥当性 | **PASS** (staged extract で段階分割 + resume 対応) |

**総合判定: PoC 成功 — 全 7 条件達成**

---

## 発見された課題の対応完了状況

| 課題 | 状態 |
|---|---|
| FTS 日本語対応 | 完了 (#14) |
| 横断検索の多様性 | 完了 (#10) |
| EPUB テキストクリーニング | 完了 (#8) |
| EPUB repair pass | 完了 |
| 重複 ingest 防止 | 完了 (#6) |
| 画像 PDF 対応 | 完了 (#16) |
| チャンクサイズ最適化 | 未対応 (config で変更可能、PoC 範囲外) |

---

## 本格実装への提言

1. **Apple Silicon 向け OCR-heavy extract の auto 化** — `extract_batch_workers` は `auto|<positive int>` の single-field 設定を維持しつつ、`extract_auto_max_workers` で上限を調整できるようにする。`auto` は Apple Silicon family 向け tier heuristic (`base/pro/max`) と warm-up 2 batch 実測で worker 数を選ぶ。現時点のこの PC では `auto` は `workers=2` を選び、fixed 比較では `workers=6..8` が最速帯だったため、`auto` は引き続き conservative、手動比較候補は `6` と `8` を残す
2. **extract / embedding の検証分離** — 日常計測は `benchmark-extract` と `benchmark-embedding` に分け、`page_extract_ms` は wall clock、`page_extract_sum_ms` は batch 合計として見比べる
3. **Embedding batch size 最適化** — text-heavy PDF の分離 benchmark (`validation-results/embedding-bench-20260326-170127`) では `nomic-embed-text` の `64` が `32` より明確に速く、`16` とほぼ同等かやや良い水準だったため、既定 batch size は `64` を採用する
4. **EPUB repair の品質しきい値調整** — repair 成功率と `text_corrupt` 判定の境界を benchmark ベースで見直す
5. **ANN インデックス** — 500冊超で sqlite-vec KNN がボトルネック化 (`docs/scale-estimation.md`)
6. **CI/CD** — PR は `make ci`、full validate は self-hosted / nightly に分離
