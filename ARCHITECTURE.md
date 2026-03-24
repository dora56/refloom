# ARCHITECTURE.md — Refloom

## 概要

Refloom はローカルファーストの読書支援 RAG ツール。
PDF/EPUB から抽出したテキストを SQLite に格納し、ハイブリッド検索 + LLM で質問応答する。

## システム構成 (C4 Level 2)

```
┌─────────────────────────────────────────────────┐
│                   CLI (cobra)                    │
│  ingest / search / ask / reindex / inspect       │
└──────┬──────┬──────┬──────┬──────┬──────────────┘
       │      │      │      │      │
       ▼      │      │      │      ▼
┌──────────┐  │      │      │  ┌──────────────┐
│extraction│  │      │      │  │  config       │
│ (Go↔Py)  │  │      │      │  │  YAML+env    │
└────┬─────┘  │      │      │  └──────────────┘
     │        │      │      │
     ▼        ▼      │      ▼
┌─────────────────┐  │  ┌──────────┐
│       db        │  │  │ embedding│
│ SQLite + FTS5   │  │  │ (Ollama) │
│ + sqlite-vec    │  │  └──────────┘
└─────────────────┘  │
                     ▼
              ┌─────────────┐
              │   search    │
              │ hybrid RRF  │
              │ intent+alias│
              └──────┬──────┘
                     │
                     ▼
              ┌─────────────┐     ┌─────────────┐
              │  citation   │────▶│    llm       │
              │ prompt build│     │ claude/ollama│
              └─────────────┘     └─────────────┘

┌─────────────────────────────────────────────────┐
│            Python Worker (subprocess)            │
│  pdf_extractor / epub_extractor / chunker        │
│  quality / ocr_vision                            │
└─────────────────────────────────────────────────┘
```

## コンポーネント

### CLI 層 (`internal/cli/`)
cobra ベースのコマンド群。ユーザー入力を受け取り、各パッケージを呼び出す。

### 抽出 (`internal/extraction/` + `python/refloom_worker/`)
Go から Python worker を subprocess で起動し、JSON プロトコルで通信。
PDF は PyMuPDF + Apple Vision OCR、EPUB は ebooklib + BeautifulSoup で処理。
抽出品質を ok/ocr_required/extract_failed/text_corrupt に分類。

### DB (`internal/db/`)
SQLite に書籍・チャプター・チャンク・embedding を格納。
FTS5 は trigram + kagome segmented の 2 テーブル構成で、BM25 スコアの高い方を採用。
sqlite-vec でベクトル近傍検索。マイグレーションは SQL ファイル + schema_version テーブルで管理。

### 検索 (`internal/search/`)
FTS5 (BM25) + sqlite-vec (cosine) を Reciprocal Rank Fusion (k=60) で統合。
比較意図検出時は書籍多様化 (round-robin)。クエリ別名展開あり。

### 引用・プロンプト (`internal/citation/`)
検索結果からプロンプトを構築。budget (総文字数) と per-chunk (チャンク文字数) で制御。

### LLM (`internal/llm/`)
claude-cli (Claude Code CLI)、anthropic (API 直接)、ollama の 3 プロバイダ。

### Embedding (`internal/embedding/`)
Ollama API で nomic-embed-text モデルを使用。

## データフロー

```
PDF/EPUB → Python Worker → JSON → Go (ingest)
  → SQLite (books, chapters, chunks)
  → Ollama embedding → sqlite-vec

Query → intent detection → FTS5 + sqlite-vec
  → RRF merge → (diversify) → citation prompt → LLM → answer
```

## 技術的決定

| 決定 | 理由 |
|---|---|
| SQLite (not Postgres) | ローカルファースト、ゼロ設定、FTS5 + sqlite-vec で十分 |
| Python subprocess | PyMuPDF/pyobjc の Go バインディングなし。JSON プロトコルで疎結合 |
| FTS5 trigram + kagome | 日本語の部分一致 (trigram) と形態素検索 (kagome) の両立 |
| RRF (not rerank) | 100冊規模では RRF で十分。rerank はスケール時に検討 |
| Apple Vision OCR | macOS ネイティブ、追加ライセンス不要、日本語精度良好 |
