# Refloom

ローカルファーストの読書支援 RAG ツール。PDF/EPUB を取り込み、ハイブリッド検索 (FTS5 + vector) と LLM で質問応答する。

全てのデータはローカルの SQLite に保存され、外部サービスへのデータ送信なし (LLM 利用時を除く)。

## 特徴

- **ハイブリッド検索**: FTS5 (trigram + kagome 形態素解析) と sqlite-vec (コサイン類似度) を RRF で統合
- **日本語対応**: kagome IPA 辞書による形態素解析で日本語テキストの検索精度を向上
- **OCR 対応**: Apple Vision Framework によるスキャン PDF の自動 OCR
- **LLM 質問応答**: 検索結果を引用付きで LLM に渡し、ソース付き回答を生成
- **高速処理**: Persistent worker pool、OCR キャッシュ、並列 embedding バッチ

## 必要環境

- macOS (Apple Silicon)
- Python 3.12+ / [uv](https://docs.astral.sh/uv/)
- [Ollama](https://ollama.com/) (embedding 生成用)

## インストール

### Homebrew (推奨)

```bash
brew tap dora56/tap
brew install refloom
```

### ソースからビルド

Go 1.24+ (CGO 有効) が必要。

```bash
git clone https://github.com/dora56/refloom.git
cd refloom
make build
cd python/refloom_worker && uv sync --group dev && cd ../..
```

### GitHub Releases

[Releases](https://github.com/dora56/refloom/releases) から darwin-arm64 zip をダウンロードして展開。

## クイックスタート

```bash
# 1. Ollama の準備
brew install ollama
ollama pull nomic-embed-text

# 2. 書籍の取り込み
refloom ingest ~/Books/example.pdf

# 3. 検索
refloom search "キーワード"

# 4. 質問応答
refloom ask "この本の主要な論点は何ですか？"
```

## コマンド一覧

### 書籍の取り込み

```bash
refloom ingest <path>                 # PDF/EPUB を取り込み
refloom ingest <dir>                  # ディレクトリ内の全 PDF/EPUB を取り込み
refloom ingest <path> --force         # 再取り込み
refloom ingest <path> --tag tech      # タグ付き取り込み
refloom ingest <path> --skip-embedding  # embedding スキップ (FTS のみ)
refloom ingest <path> --profile-json  # パフォーマンスプロファイル出力
```

### 検索

```bash
refloom search "ドメインモデリング"           # ハイブリッド検索 (デフォルト)
refloom search --mode keyword "キーワード"   # FTS のみ
refloom search --mode vector "意味検索"      # ベクトルのみ
refloom search --json "クエリ"               # JSON 出力
refloom search --limit 20 "クエリ"           # 結果数指定
```

### 質問応答

```bash
refloom ask "この本の主要な論点は何ですか？"
refloom ask --json "質問"                    # JSON 出力 (タイミング情報付き)
refloom ask --expand-context "質問"          # 前後チャンクも含めて回答 (ADR-0010)
refloom ask --hyde "複雑な質問"              # HyDE で語彙ミスマッチを改善 (ADR-0012)
```

### 管理コマンド

```bash
refloom inspect                  # DB 内の書籍一覧
refloom delete <book_id>         # 書籍の削除
refloom reindex                  # FTS/embedding の再構築
refloom reindex --binary         # バイナリベクトルインデックス構築 (ADR-0011)
refloom export --format json     # データエクスポート
refloom doctor                   # システムヘルスチェック
refloom config-show              # 現在の設定を表示
refloom work prune --dry-run     # 作業ディレクトリの掃除 (プレビュー)
refloom version                  # バージョン情報
```

## 設定

`~/.refloom/config.yaml` または環境変数で設定。テンプレート: `config/refloom.example.yaml`

| 設定 | 環境変数 | デフォルト |
|---|---|---|
| `db_path` | `REFLOOM_DB_PATH` | `~/.refloom/refloom.db` |
| `ollama_url` | `REFLOOM_OLLAMA_URL` | `http://localhost:11434` |
| `ollama_embedding_model` | `REFLOOM_EMBEDDING_MODEL` | `nomic-embed-text` |
| `embedding_batch_size` | `REFLOOM_EMBEDDING_BATCH_SIZE` | `64` |
| `embed_parallel_workers` | `REFLOOM_EMBED_PARALLEL_WORKERS` | `2` |
| `extract_batch_workers` | `REFLOOM_EXTRACT_BATCH_WORKERS` | `auto` |
| `llm_provider` | `REFLOOM_LLM_PROVIDER` | `claude-cli` |
| `chunk_size` | — | `500` |
| `chunk_overlap` | — | `100` |

## 検証

```bash
make ci               # lint + test (Go + Python)
make validate          # E2E 検証 (Ollama 必要)
make validate-fresh    # クリーン DB での E2E 検証
```

## トラブルシューティング

### `refloom doctor` でチェック

```bash
refloom doctor
```

以下を自動検証: DB 整合性、Ollama 接続、Python worker、ディスク使用量、OCR キャッシュ。

### よくある問題

| 症状 | 対処 |
|------|------|
| `ollama not reachable` | `ollama serve` を起動 |
| `model not found` | `ollama pull nomic-embed-text` |
| `python not found` | `cd python/refloom_worker && uv sync` |
| embedding が遅い | `embed_parallel_workers: 4` に増やす |
| OCR が遅い | 2回目以降はキャッシュが効く (`~/.refloom/cache/ocr/`) |

## アーキテクチャ

Go (CLI/検索/DB) + Python (抽出/OCR/チャンキング) の 2 言語構成。
詳細は [ARCHITECTURE.md](ARCHITECTURE.md)、設計判断は [docs/adr/](docs/adr/) を参照。

## ライセンス

[MIT](LICENSE)

### サードパーティライセンス

- **PyMuPDF**: AGPL-3.0 / Commercial dual license。Refloom は PyMuPDF を subprocess 経由で使用しており、リンクしていません。
- その他の依存ライブラリは MIT, BSD, Apache 2.0 等のオープンソースライセンスです。
