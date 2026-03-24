# Refloom PoC 完了レポート

## 概要

Refloom はローカルファーストの CLI 読書支援ツールである。PDF/EPUB の個人蔵書に対して、テキスト抽出・インデックス・検索・LLM による質問応答を提供する。

本 PoC は「コンパクトなローカルスタックで実用的な読書支援が可能か」を検証し、**全 7 成功条件を達成して PoC を完了**した。

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
│ Python Worker (subprocess + JSON)          │
│  pdf_extractor (PyMuPDF + Vision OCR)     │
│  epub_extractor (ebooklib + clean_text)   │
│  chunker (paragraph-aware)                │
│  quality (text corruption detection)      │
│  ocr_vision (Apple Vision Framework)      │
├────────────────────────────────────────────┤
│ SQLite (mattn/go-sqlite3)                  │
│  book, chapter, chunk (with prev/next)    │
│  chunk_fts (trigram) + chunk_fts_seg      │
│  chunk_vec (sqlite-vec, float[768])       │
│  ingest_log, schema_version               │
├────────────────────────────────────────────┤
│ Ollama (embeddinggemma)  │ Claude CLI     │
└──────────────────────────┴─────────────────┘
```

---

## 最終検証結果

検証日: 2026-03-24 | コーパス: 6冊 (PDF 3 + EPUB 3)

| 指標 | 結果 |
|---|---|
| 有効書籍 | **6/6** (画像PDF含む) |
| 総チャンク数 | 3,783 |
| Keyword hit rate (top-5) | **9/12** |
| Hybrid hit rate (top-5) | **10/12** |
| Ask ソース付き回答 | **3/3** |
| Ask median total latency | 15.4 秒 |
| Ask median retrieval latency | 244 ms |
| Ask median generation latency | 15.2 秒 |

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
- テストコード (Python 34件 + Go)

---

## PoC 成功条件の達成状況

| # | 条件 | 判定 |
|---|---|---|
| 1 | EPUB/PDF から実用レベルで抽出できる | **PASS** (6/6冊、OCR含む) |
| 2 | 3〜5冊に対して検索が有効に機能する | **PASS** (hybrid 10/12) |
| 3 | `ask` で要約中心の有用な回答が返る | **PASS** (3/3) |
| 4 | 回答に本名・章名・ページ範囲を含められる | **PASS** |
| 5 | 長文引用を抑えられる | **PASS** |
| 6 | M2 MacBook Pro 上で実用的に動作する | **PASS** |
| 7 | 本体 + Python ワーカー構成の妥当性 | **PASS** |

**総合判定: PoC 成功 — 全 7 条件達成**

---

## 発見された課題の対応完了状況

| 課題 | 状態 |
|---|---|
| FTS 日本語対応 | 完了 (#14) |
| 横断検索の多様性 | 完了 (#10) |
| EPUB テキストクリーニング | 完了 (#8) |
| 重複 ingest 防止 | 完了 (#6) |
| 画像 PDF 対応 | 完了 (#16) |
| チャンクサイズ最適化 | 未対応 (config で変更可能、PoC 範囲外) |

---

## 本格実装への提言

1. **EPUB text_corrupt の自動修復** — 検出はできるが修復パイプラインがない
2. **Embedding モデル評価** — embeddinggemma vs nomic-embed-text の品質比較
3. **ANN インデックス** — 500冊超で sqlite-vec KNN がボトルネック化 (`docs/scale-estimation.md`)
4. **CI/CD** — `make validate` を GitHub Actions で自動実行
5. **Web UI** — CLI に加えてブラウザベースの検索・閲覧インターフェース
