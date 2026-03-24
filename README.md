# Refloom

ローカルファーストの読書支援 RAG ツール。PDF/EPUB を取り込み、ハイブリッド検索と LLM で質問応答する。

## 必要環境

- macOS (Apple Silicon)
- Go 1.26+ (CGO 有効)
- Python 3.12+ / [uv](https://docs.astral.sh/uv/)
- [Ollama](https://ollama.com/) (embedding 用)

## セットアップ

```bash
# Ollama モデルの取得
ollama pull nomic-embed-text

# ビルド
make build

# Python 依存のインストール
cd python/refloom_worker && uv sync --group dev && cd ../..

# 設定ファイル (任意)
cp config/refloom.example.yaml ~/.refloom/config.yaml
# 必要に応じて編集
```

## 使い方

### 書籍の取り込み

```bash
refloom ingest ~/Books/example.pdf
refloom ingest ~/Books/example.epub
```

### 検索

```bash
refloom search "キーワード"
refloom search --json "キーワード"    # JSON 出力
```

### 質問応答

```bash
refloom ask "この本の主要な論点は何ですか？"
refloom ask --json "質問"             # JSON 出力 (タイミング情報付き)
```

### その他のコマンド

```bash
refloom inspect                       # DB 内の書籍一覧
refloom reindex                       # FTS/embedding の再構築
refloom reindex --links               # チャンクリンクの再構築
refloom version                       # バージョン情報
refloom help                          # ヘルプ
```

## 設定

`~/.refloom/config.yaml` または環境変数で設定。

| 設定 | 環境変数 | デフォルト |
|---|---|---|
| `db_path` | `REFLOOM_DB_PATH` | `~/.refloom/refloom.db` |
| `python_worker_dir` | `REFLOOM_WORKER_DIR` | (自動検出) |
| `ollama_url` | `REFLOOM_OLLAMA_URL` | `http://localhost:11434` |
| `ollama_embedding_model` | `REFLOOM_EMBEDDING_MODEL` | `nomic-embed-text` |
| `llm_provider` | `REFLOOM_LLM_PROVIDER` | `claude-cli` |
| `anthropic_api_key` | `ANTHROPIC_API_KEY` | (未設定) |

## アーキテクチャ

詳細は [ARCHITECTURE.md](ARCHITECTURE.md) を参照。

## ライセンス

Private
