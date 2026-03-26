# ADR-0002: Go + Python デュアル言語アーキテクチャの採用

- Status: Accepted
- Date: 2026-03-27
- Deciders: Refloom maintainers

## Context

Refloom は PDF/EPUB からのテキスト抽出・OCR・チャンキングと、CLI・DB 操作・検索・LLM 連携という二つの性質の異なる処理を持つ。抽出側は PyMuPDF・ebooklib・pyobjc (Apple Vision OCR) といった Python エコシステムに成熟したライブラリがあるが、いずれも Go バインディングが存在しない。一方 CLI・DB・検索は Go のシングルバイナリ配布と CGO (go-sqlite3, sqlite-vec) の相性が良い。

純 Go 構成、純 Python 構成、Go + Python 混成の 3 案を検討した。

## Decision

Go を CLI・オーケストレーション・DB・検索・embedding クライアントに、Python を抽出・OCR・チャンキングの worker に採用する。Go が Python をサブプロセスとして起動し、stdin/stdout 上の JSON プロトコル（1 リクエスト → 1 レスポンス）で IPC する。worker はステートレスで、コマンドごとに独立した処理を行う。

## Consequences

### Positive

- 各言語の成熟したエコシステムを活用できる（PyMuPDF, pyobjc, cobra, go-sqlite3）
- JSON プロトコルにより Go/Python を疎結合に保ち、独立してテスト・デバッグできる
- Go シングルバイナリ + Python venv で配布構成がシンプル

### Negative

- サブプロセス起動のオーバーヘッドがコマンドごとに発生する
- go.mod と pyproject.toml の二重依存管理が必要になる
- 障害調査時に Go/Python 双方のログを突き合わせる必要がある

### Neutral

- Python worker はステートレスなため、将来的な並列 worker 化が容易
- プロトコルのバージョニングはコマンドセットの変更で暗黙的に管理される
