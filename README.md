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
refloom ingest ~/Books/example.pdf --profile-json
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

### 検証とベンチマーク

```bash
make validate
make validate-fresh
make validate-ingest
refloom work prune --dry-run
make benchmark-extract
make benchmark-embedding
BENCH_BOOK_FILTER='マルチテナント' BENCH_EXTRACT_WORKERS='1,2,4,6,8,auto' make benchmark-extract
BENCH_EMBEDDING_MODELS=nomic-embed-text BENCH_EMBEDDING_BATCH_SIZES=16,32,64 make benchmark-embedding
REFLOOM_EXTRACT_BATCH_WORKERS=2 VALIDATE_BOOK_FILTER='マルチテナント' make validate-ingest
VALIDATE_FRESH_DB=1 VALIDATE_SKIP_SEARCH=1 VALIDATE_SKIP_ASK=1 VALIDATE_SKIP_SCORE=1 VALIDATE_BOOK_FILTER='マルチテナント|データモデリング' make validate
python3 scripts/estimate_extract_parallelism.py ~/.refloom/work/<job-id>/manifest.json
```

- 既定の embedding モデルは `nomic-embed-text`
- embedding batch size は `REFLOOM_EMBEDDING_BATCH_SIZE` または `embedding_batch_size` で切り替え可能
- 既定の embedding batch size は `64`。text-heavy PDF の分離 benchmark では `nomic-embed-text` で `32` より明確に速く、`16` と同等以上だった
- `extract_batch_workers` / `REFLOOM_EXTRACT_BATCH_WORKERS` は `auto` または正の整数を受け付ける。既定値は `auto`
- `extract_auto_max_workers` / `REFLOOM_EXTRACT_AUTO_MAX_WORKERS` は Apple Silicon 向け `auto` heuristic の上限を決める。既定値は `8`
- `auto` は Apple Silicon family 向けの tier heuristic で、`perf_cores` から `base/pro/max` tier を決め、`extract_auto_max_workers`・メモリ safety・warm-up 2 batch 実測で最終 worker 数を丸める
- `workers>=2` は OCR-heavy PDF にのみ適用される。EPUB と text-heavy PDF は `workers=1` のまま動く
- この PC の OCR-heavy benchmark では `auto` は `workers=2` を選び、固定比較では `workers=6` と `workers=8` が最速帯だった。`auto_worker_reason` を見ると `avg_batch_ms=4093` の warm-up が `2` を選んだことが確認できる
- OCR チューニングの比較時は `REFLOOM_OCR_FAST_SCALE` と `REFLOOM_OCR_RETRY_MIN_CHARS` を使う
- extract は `probe -> extract-pages -> chunk` の staged worker で動き、中間成果物は `~/.refloom/work/<job-id>/` に保存される
- `--profile-json` では `probe_ms`, `page_extract_ms` (wall clock), `page_extract_sum_ms` (batch 合計), `chunk_ms`, `batch_count`, `failed_batch_count`, `extract_workers_used`, `extract_auto_max_workers`, `extract_auto_effective_cap`, `extract_auto_tier`, `extract_auto_candidates`, `parallel_extract_enabled`, `resumed`, `job_dir` を確認できる
- `benchmark-extract` は extract の worker 数比較専用で、既定比較セットは `1,2,4,6,8,auto`
- `benchmark-embedding` は embedding のモデル / batch size 比較専用
- 日常の最小性能確認は `make validate-ingest` または `VALIDATE_BOOK_FILTER` / `VALIDATE_MAX_BOOKS` / `VALIDATE_SKIP_*` を使って対象を絞る
- 正式な性能 baseline は fresh DB の `make validate-fresh` で取る。現行 baseline は [validation-results/20260326-075122](./validation-results/20260326-075122) で、`effective_books=6/6`, `total_chunks=3377`, `6冊 total ingest=2299764ms`
- `refloom work prune` は `~/.refloom/work` の completed job を掃除する。既定では failed / resumable job は消さない

## 設定

`~/.refloom/config.yaml` または環境変数で設定。

| 設定 | 環境変数 | デフォルト |
|---|---|---|
| `db_path` | `REFLOOM_DB_PATH` | `~/.refloom/refloom.db` |
| `python_worker_dir` | `REFLOOM_WORKER_DIR` | (自動検出) |
| `ollama_url` | `REFLOOM_OLLAMA_URL` | `http://localhost:11434` |
| `ollama_embedding_model` | `REFLOOM_EMBEDDING_MODEL` | `nomic-embed-text` |
| `embedding_batch_size` | `REFLOOM_EMBEDDING_BATCH_SIZE` | `64` |
| `extract_batch_workers` | `REFLOOM_EXTRACT_BATCH_WORKERS` | `auto` |
| `extract_auto_max_workers` | `REFLOOM_EXTRACT_AUTO_MAX_WORKERS` | `8` |
| `llm_provider` | `REFLOOM_LLM_PROVIDER` | `claude-cli` |
| `anthropic_api_key` | `ANTHROPIC_API_KEY` | (未設定) |

## アーキテクチャ

詳細は [ARCHITECTURE.md](ARCHITECTURE.md) を参照。

## ライセンス

Private
