# ADR-0007: Persistent Worker Pool

- Status: Accepted
- Date: 2026-03-27
- Deciders: dora56

## Context

Go CLI は Python worker を subprocess として呼び出し、probe / extract-pages / chunk の各コマンドを実行する。
現在の実装 (`runJSONCommand`) は**コマンドごとに新しい Python プロセスを起動・終了する** fire-and-forget モデル。

455 ページ OCR-heavy PDF の場合、1 冊の ingest で 31 回の subprocess spawn が発生する:
- 1 probe + 29 extract-pages バッチ + 1 chunk

各 spawn のオーバーヘッド (~600ms):
- Python インタープリタ起動: 200-300ms
- モジュール import (PyMuPDF, Vision etc.): 150-200ms
- fitz.Document.open(): 50-100ms
- JSON I/O: 50ms

合計: **31 spawns × 600ms = ~18.6 秒 (全体の 14%)**

## Decision

**Persistent stdin/stdout worker pool** を採用する。

Python worker に `--persistent` フラグを追加し、stdin から改行区切り JSON コマンドを読み続けるループモードを実装する。Go 側は `PersistentWorker` 構造体で N 個の長寿命プロセスをプールし、既存の `Extractor` インターフェース経由で透過的に利用する。

### 検討した選択肢

| 選択肢 | 説明 | 判定 |
|---|---|---|
| A) 現状維持 (spawn-per-call) | 改善なし | 却下 |
| **B) Persistent worker pool** | stdin/stdout 長寿命プロセス | **採用** |
| C) Unix domain socket / gRPC | プロセス間通信を高度化 | 過剰 |

### 採用理由

- 既存 JSON プロトコルを改行区切りに変えるだけで実装コストが低い
- `Extractor` インターフェース (ADR-0001 で導入) 経由で後方互換性を維持
- spawn-per-call モードもフォールバックとして残せる
- PDF ドキュメントのインプロセスキャッシュも可能になる

## Consequences

### Positive

- subprocess spawn オーバーヘッドを ~18.6 秒 → ~1.2 秒に削減 (2 プロセス起動のみ)
- PyMuPDF ドキュメントのインプロセスキャッシュで fitz.open() コストも削減
- Python import コストが初回のみに

### Negative

- 長寿命プロセスのメモリリーク管理が必要 (N コマンド後の recycle を検討)
- プロセスクラッシュ時の回復ロジックが必要
- stdin/stdout のデッドロック防止に注意 (flush 必須)

### Neutral

- プロトコルの JSON 構造自体は変更なし (フレーミングのみ改行区切りに)
- single-shot モードは互換性のため残す
