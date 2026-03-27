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
│(pool/IPC)│  │      │      │  │  YAML+env    │
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
│       Python Worker (persistent pool / subprocess) │
│  pdf_extractor / epub_extractor / chunker          │
│  quality / ocr_vision / OCR cache                  │
└─────────────────────────────────────────────────┘
```

## コンポーネント

### CLI 層 (`internal/cli/`)
cobra ベースのコマンド群。ユーザー入力を受け取り、各パッケージを呼び出す。

### 抽出 (`internal/extraction/` + `python/refloom_worker/`)
Go から Python worker を persistent pool (改行区切り JSON on stdin/stdout) で管理。
起動時に N 個のワーカーを起動し再利用、クラッシュ時は自動 respawn。
spawn-per-call にフォールバック可能。(ADR-0007)
PDF は PyMuPDF + Apple Vision OCR、EPUB は ebooklib + BeautifulSoup で処理。
OCR-heavy 判定時は accurate-only ポリシーで Vision API 呼び出しを半減。
ページ単位の OCR キャッシュを `~/.refloom/cache/ocr/` に保存。(ADR-0008)
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
Ollama API で nomic-embed-text (768次元) を使用。
2-4 並列ゴルーチンでバッチ送信、DB 保存は逐次 (SQLite 制約)。(ADR-0009)

## データフロー

```
PDF/EPUB → Python Worker (persistent pool) → JSONL → Go (staged extract)
  → SQLite (books, chapters, chunks) + FTS5 (trigram + kagome)
  → Ollama embedding (parallel batch) → sqlite-vec

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
| Persistent worker pool | subprocess spawn 14% → 1% に削減 (ADR-0007) |
| OCR accurate-only + cache | OCR-heavy で Vision 呼び出し半減、再 ingest で OCR スキップ (ADR-0008) |
| Embedding 並列バッチ | Ollama HTTP I/O 待ち 95% を並列化で 50-75% 削減 (ADR-0009) |
