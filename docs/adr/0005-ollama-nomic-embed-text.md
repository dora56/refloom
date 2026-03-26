# ADR-0005: Ollama nomic-embed-text によるローカル Embedding

- Status: Accepted
- Date: 2026-03-27
- Deciders: Refloom maintainers

## Context

ハイブリッド検索のセマンティック検索側に embedding モデルが必要だった。OpenAI Embedding API (クラウド、有料) とローカル Ollama の 2 方式を検討した。ローカル候補として nomic-embed-text と embeddinggemma を 6 冊コーパスでベンチマークした。

ベンチマーク結果:
- nomic-embed-text: 全 embedding 生成 **2.24M ms**、keyword hit 9/12、hybrid hit 10/12
- embeddinggemma: 全 embedding 生成 **3.40M ms**、keyword hit 9/12、hybrid hit 10/12

検索品質は同等だが、nomic が 34% 高速だった。

## Decision

Ollama nomic-embed-text (768 次元) をデフォルト embedding モデルとして採用する。Ollama HTTP API (`/api/embed`) 経由でバッチ embedding を行い、バッチサイズはデフォルト 64 とする。リトライは指数バックオフ (100ms → 500ms → 2s) で 3 回まで。モデル名は設定で変更可能とする。

## Consequences

### Positive

- ローカル実行のためプライバシーが保たれ、API コストが発生しない
- nomic-embed-text は embeddinggemma と同等品質で 34% 高速
- `embedding_version` カラムでモデル変更時の追跡が可能

### Negative

- Ollama のローカル起動が前提となり、初回セットアップのハードルがある
- 768 次元ベクトルが DB サイズの約 85% を占め、ストレージ効率が低い
- 商用 API (OpenAI, Cohere) と比較するとモデル品質に上限がある

### Neutral

- ベンチマーク詳細は [docs/reports/poc-final-report.md](../reports/poc-final-report.md) の embedding モデル比較セクションを参照
