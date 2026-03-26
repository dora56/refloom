# ADR-0006: LLM マルチプロバイダと出典バジェット制御

- Status: Accepted
- Date: 2026-03-27
- Deciders: Refloom maintainers

## Context

`ask` コマンドの回答生成に LLM が必要だが、ユーザーの環境によって利用可能な LLM が異なる。Claude Code CLI がインストールされている環境、Anthropic API キーを持つ環境、Ollama のみの環境をすべてサポートしたい。

また、検索結果の出典テキストをそのまま LLM プロンプトに含めるとコンテキストウィンドウを圧迫するため、プロンプトサイズの制御が必要だった。

## Decision

`internal/llm/llm.go` に `Provider` インターフェースを定義し、3 つの実装を用意する。

- **claude-cli**: `claude --print` コマンド経由。API キー不要でデフォルトプロバイダ
- **anthropic**: Anthropic HTTP API 直接呼び出し。API キー必要
- **ollama**: ローカル Ollama HTTP API

出典バジェットは `budget=3000` 文字（全体）、`per_chunk=500` 文字（チャンクごと上限）のハード制限を設ける。バジェット超過時は末尾のチャンクを切り捨てる。出典フォーマットは `[番号] 書籍名, 章名, pp.X-Y` で統一する。

## Consequences

### Positive

- API キーなしでも claude-cli 経由で動作し、環境構築のハードルが低い
- プロバイダ切り替えが設定ファイル 1 行で完結する
- バジェット制御により LLM へのプロンプトサイズが予測可能になる

### Negative

- claude-cli は Claude Code の外部インストールに依存する
- バジェット切り詰めにより関連性の高い出典が欠落する可能性がある
- プロバイダごとにプロンプト最適化が異なる可能性があるが、現在は共通プロンプトで統一している

### Neutral

- プロバイダ選択は `llm_provider` 設定 (環境変数 `REFLOOM_LLM_PROVIDER`) で行う
- バジェット値は設定で変更可能だが、デフォルト値で PoC 品質は十分だった
