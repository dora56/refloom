# Refloom PoC 実装比較レポート: Claude Code (main) vs Codex (worktree)

作成日: 2026-03-24

---

## 1. 概要

同一の PoC 仕様に対して、Claude Code CLI と Codex CLI の2つの AI エージェントがそれぞれ独立に Refloom を実装した。本レポートは両実装の設計判断・品質・検証手法を比較し、相互に学べる点を整理する。

---

## 2. 実装スコープの比較

| 観点 | Claude Code (main) | Codex (worktree) |
|---|---|---|
| コミット数 | 8 (運用改善 #1〜#8 含む) | 1 (単一コミット) |
| Go 依存 | cobra, go-sqlite3, sqlite-vec | golang.org/x/text のみ |
| SQLite 操作 | Go ドライバ (mattn/go-sqlite3) | **sqlite3 CLI を exec で呼び出し** |
| ベクトル検索 | sqlite-vec 拡張 (ネイティブ) | JSON 配列 + Go 側で cosine 計算 |
| PDF 抽出 | PyMuPDF | pdftotext (poppler) |
| EPUB 抽出 | ebooklib + BeautifulSoup | zipfile + XML パーサ (標準ライブラリのみ) |
| LLM | Claude Code CLI (`claude --print`) | Ollama ローカルモデル (qwen2.5:7b) |
| 設定ファイル | 環境変数ベース | TOML ファイル |
| チャンクサイズ | 500文字 / 100文字 overlap | 1000文字 / 150文字 overlap |

---

## 3. アーキテクチャ比較

### 3.1 Go パッケージ構成

| Claude Code (main) | Codex (worktree) |
|---|---|
| `cmd/refloom/` — cobra ベース | `cmd/refloom/` — 独自 `app.Run()` |
| `internal/cli/` — 各サブコマンド | `internal/app/` — **1498行の単一ファイル** |
| `internal/db/` — DB 層 | `internal/sqlite/` — sqlite3 CLI ラッパー |
| `internal/embedding/` — Ollama client | `internal/ollama/` — Ollama client |
| `internal/extraction/` — worker 呼び出し | `internal/worker/` — worker 呼び出し |
| `internal/config/` — 環境変数パース | `internal/config/` — TOML パース |
| `internal/search/` — 検索ロジック | (app.go に統合) |
| `internal/llm/` — LLM 呼び出し | (app.go に統合) |
| `internal/citation/` — 出典生成 | (app.go に統合) |

**評価**: Claude Code 版はパッケージが細かく分離されており保守性が高い。Codex 版は `app.go` に多くが集中しているが、PoC としては一覧性が良い。

### 3.2 DB アクセス方式

- **Claude Code**: Go ドライバ (`mattn/go-sqlite3`) + sqlite-vec 拡張でネイティブベクトル検索。CGO 必須。
- **Codex**: `sqlite3` CLI を `exec.CommandContext` で呼び出し。JSON モード出力をパース。CGO 不要だが、sqlite3 バイナリへの依存と exec オーバーヘッドあり。

**評価**: Claude Code 版の方がパフォーマンス・型安全性で優位。Codex 版は CGO 回避のトレードオフとして理解できるが、プロダクション向けではない。

---

## 4. 検索・回答品質の比較

### 4.1 検索精度

| 指標 | Claude Code (main) | Codex (worktree) |
|---|---|---|
| 検証書籍数 | 6冊 (5冊有効) | 4冊 (1冊有効 + 1 OCR + 2 text_corrupt) |
| Keyword hit rate | 未定量化 (手動確認 OK) | 10/10 adjusted (100%) |
| Hybrid hit rate | 全クエリ OK (手動確認) | 10/10 adjusted (100%) |
| クエリセット | 5 クエリ (手動) | **12 クエリ (構造化 JSON)** |

### 4.2 回答品質

| 指標 | Claude Code (main) | Codex (worktree) |
|---|---|---|
| LLM | Claude (`claude --print`) | qwen2.5:7b (ローカル) |
| ask 成功率 | 3/3 PASS | 8/8 sources present |
| ask レイテンシ | 14〜15秒 | median 10.1秒 / p95 12.7秒 |
| プロンプト制御 | 300文字切り詰め + システムプロンプト | **950文字バジェット + 120文字/チャンク上限** |

**評価**: Codex 版はプロンプトバジェット管理がより精緻。ローカル LLM でのレイテンシも良好。

---

## 5. 検証手法の比較

| 観点 | Claude Code (main) | Codex (worktree) |
|---|---|---|
| 検証方式 | 手動実行 + 評価メモ (Markdown) | **自動スクリプト + タイムスタンプ付きアーティファクト** |
| クエリ管理 | ドキュメント内に記述 | **query-set.json で構造化** |
| スコアリング | 手動判定 (OK/NG) | **score_validation.py で自動計算** |
| 抽出品質分類 | OK / NG (画像PDF) | **ok / ocr_required / extract_failed / text_corrupt の4段階** |
| テキスト破損検出 | なし | **`looks_text_corrupt()` でサンプリング検出** |
| 再現性 | 低 (手動手順) | **高 (validate_refloom.sh で完全再現)** |

**評価**: **Codex 版の検証手法は明確に優れている。** 自動化されたバリデーションパイプライン、構造化されたクエリセット、タイムスタンプ付きアーティファクトは、PoC の信頼性を大幅に高めている。

---

## 6. 運用堅牢性の比較

| 機能 | Claude Code (main) | Codex (worktree) |
|---|---|---|
| 構造化ログ (slog) | あり (#1) | なし |
| Embedding リトライ | あり (#2, 指数バックオフ) | なし |
| タイムアウト設定化 | あり (#3) | 3分固定 |
| Worker 堅牢化 | あり (#4) | なし |
| DB マイグレーション管理 | あり (#5) | なし |
| ファイルハッシュ重複防止 | あり (#6) | source_path UNIQUE のみ |
| バージョン情報 | あり (#7) | なし |
| EPUB テキストクリーニング | あり (#8) | 最低限 (改行圧縮のみ) |

**評価**: Claude Code 版は #1〜#8 の運用改善で大幅にプロダクション寄りになっている。Codex 版はこれらが全くないが、PoC 段階としては妥当。

---

## 7. Codex 版から学ぶべき点

### 7.1 検索の工夫 (採用推奨)

- **比較意図検出**: `比較`, `共通点`, `違い` 等の制御語を検出し、複数書籍から結果を分散させる
- **キーワードエイリアス**: `文章品質` → `文書`, `品質`, `読みやすさ` の同義語展開
- **書籍プロファイルフォールバック**: 1冊しかヒットしない場合に他書籍の代表チャンクを注入
- **段階的フォールバック**: FTS phrase → FTS OR → LIKE %term% の3段階

### 7.2 検証インフラ (採用推奨)

- **validate_refloom.sh**: ワンコマンドで全検証を実行・アーティファクト生成
- **query-set.json**: 検証クエリの構造化・再利用可能化
- **score_validation.py**: 定量スコアリングの自動化
- **抽出品質の4段階分類**: ok / ocr_required / extract_failed / text_corrupt

### 7.3 プロンプトバジェット管理

- チャンクあたり120文字上限 + 総計950文字バジェットの明示的制御

### 7.4 チャンクリンク

- `prev_chunk_id` / `next_chunk_id` で前後文脈への参照を保持

---

## 8. Claude Code 版の優位点

- パッケージ分離による保守性
- ネイティブ DB アクセス (CGO + sqlite-vec)
- 運用堅牢性 (#1〜#8 の改善)
- Python worker の依存管理 (pyproject.toml + uv)
- テストコード (epub_extractor のユニットテスト)

---

## 9. 推奨アクション

| # | アクション | 元 | 優先度 |
|---|---|---|---|
| 1 | 比較意図検出 + 書籍分散を検索ロジックに導入 | Codex | 高 |
| 2 | validate_refloom.sh 相当の自動検証パイプライン構築 | Codex | 高 |
| 3 | query-set.json + score_validation.py 導入 | Codex | 高 |
| 4 | 抽出品質の4段階分類 + text_corrupt 検出 | Codex | 中 |
| 5 | チャンクリンク (prev/next) 導入 | Codex | 中 |
| 6 | プロンプトバジェットの明示的制御 | Codex | 中 |
| 7 | キーワードエイリアス機能 | Codex | 低 |

---

## 10. 総合評価

両実装は同一仕様に対する異なるアプローチであり、それぞれ明確な強みを持つ。

- **Claude Code 版**: 運用品質・保守性・堅牢性に優れ、プロダクション方向への拡張に適している
- **Codex 版**: 検索の賢さ・検証の自動化・品質分類に優れ、PoC としての完成度が高い

**最も価値のある知見**: Codex 版の検証パイプライン (自動スクリプト + 構造化クエリ + 定量スコアリング) は、Claude Code 版に最も欠けている部分であり、早急に取り込むべきである。
