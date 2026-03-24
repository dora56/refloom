# CLAUDE.md — Refloom

## プロジェクト概要

ローカルファースト CLI RAG ツール。PDF/EPUB を取り込み、SQLite に保存し、ハイブリッド検索 + LLM で質問応答する。
Go (CLI・検索・DB) + Python (抽出・チャンキング・OCR) の 2 言語構成。Apple Silicon macOS 専用。

## ビルド・テスト

```bash
# Go ビルド (CGO 必須: SQLite FTS5 + sqlite-vec)
make build          # → bin/refloom

# Go テスト
make test           # CGO_ENABLED=1 go test -tags fts5 ./...

# Go lint
make lint           # golangci-lint (govet, errcheck, staticcheck, unused, gosec, gofmt)

# Python lint
make lint-python    # ruff check + pyright

# Python テスト (uv を使用。pip は使わない)
make test-python    # uv run --group dev pytest

# 全チェック (CI 相当)
make ci             # lint + test + lint-python + test-python

# E2E 検証 (Ollama 起動が前提。CI では実行不可)
make validate
```

## コーディング規約

### Go
- `gofmt` 準拠
- ビルドタグ: `-tags fts5` 必須 (全ビルド・テストコマンドに付与)
- CGO_ENABLED=1 必須 (go-sqlite3, sqlite-vec)
- エラーは呼び出し元に返す。`log.Fatal` は main/CLI 層のみ

### Python
- Python 3.12+
- 依存管理: `uv` (`pyproject.toml` の `[dependency-groups]` を使用)
- pyobjc (Vision/Quartz) は macOS 限定依存

### コミット
- Conventional Commits スタイル (例: `Add chunk linking`, `Fix FTS regression`)
- 動詞始まり、英語、簡潔に

## ディレクトリ構成

```
cmd/refloom/          エントリーポイント
internal/
  cli/                cobra コマンド (ingest, search, ask, reindex, inspect, version)
  config/             YAML + env var 設定
  db/                 SQLite 操作、FTS5 (trigram + kagome segmented)、マイグレーション
  search/             ハイブリッド検索、intent 検出、book 多様化、alias 展開
  citation/           LLM プロンプト構築 (budget 制御)
  embedding/          Ollama embedding クライアント
  extraction/         Python worker 呼び出しプロトコル
  llm/                LLM プロバイダ (claude-cli, anthropic, ollama)
python/refloom_worker/
  main.py             worker エントリーポイント (JSON プロトコル)
  pdf_extractor.py    PyMuPDF + Vision OCR
  epub_extractor.py   ebooklib + テキストクリーニング
  chunker.py          テキストチャンキング
  quality.py          抽出品質分類
  ocr_vision.py       Apple Vision Framework ラッパー
scripts/              検証スクリプト
docs/reports/         PoC 評価・比較レポート
config/               設定ファイルテンプレート
testdata/             検証用クエリセット
```

## 重要な技術制約

- **CGO 必須**: go-sqlite3 と sqlite-vec が C ライブラリ。`CGO_ENABLED=0` ではビルド不可
- **Apple Silicon ターゲット**: Vision Framework OCR は macOS 限定。クロスコンパイルは darwin-arm64 のみ
- **Ollama 依存**: embedding 生成と一部 LLM 呼び出しに Ollama が必要。`make validate` はローカル実行のみ
- **DB マイグレーション**: `internal/db/migrations/` に SQL ファイル。`schema_version` テーブルで管理

## ブランチ戦略・レビュー

- `main`: リリースブランチ。PR 経由でマージ
- `feature/*`, `fix/*`: 開発ブランチ → PR → main
- PR は `make ci` 全パス必須
- AI レビュー: Claude Code で実装 → 別エージェント (worktree) でクロスレビュー

## テスト方針

- **TDD で進める**: 新機能・バグ修正はテストを先に書いてから実装する
- 変更したパッケージのテストは必ず通すこと (`make test`)
- Python テストは `uv run --group dev pytest` で実行
- `make validate` は手動 E2E 検証 (Ollama + 書籍データ必要)
- テストで外部サービス (Ollama, LLM API) に依存しない単体テストを優先
