# ADR-0004: Apple Vision Framework による OCR

- Status: Accepted
- Date: 2026-03-27
- Deciders: Refloom maintainers

## Context

スキャン PDF や画像ベース PDF からテキストを抽出するために OCR が必要だった。選択肢として Tesseract (GPL, クロスプラットフォーム)、Apple Vision Framework (macOS ネイティブ)、クラウド OCR API (Google Cloud Vision 等) を検討した。Refloom は個人利用の macOS Apple Silicon 専用ツールであり、クロスプラットフォーム対応は不要である。

## Decision

Apple Vision Framework を pyobjc 経由で採用する。高速認識 (1.5x スケール) → 精度認識 (2.0x スケール) の dual-scale 戦略を取り、初回で十分な文字数が得られない場合にリトライする。スケール値と最小文字数閾値は環境変数 (`REFLOOM_OCR_FAST_SCALE`, `REFLOOM_OCR_RETRY_MIN_CHARS`) で調整可能とする。

## Consequences

### Positive

- 日本語 OCR 精度が高く、特に技術書のスキャンで実用的な品質を得られた
- macOS ネイティブのため追加ライセンス・外部サービスが不要
- Apple Silicon 上で高速に動作する

### Negative

- macOS 限定であり、Linux / Windows では動作しない
- CI 環境 (Linux) で OCR を含むテストを実行できない
- pyobjc 依存により Python 側のセットアップが macOS 固有になる

### Neutral

- 将来クロスプラットフォーム対応が必要になった場合、Tesseract をフォールバックとして追加することは可能
