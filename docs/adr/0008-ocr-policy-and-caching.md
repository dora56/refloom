# ADR-0008: OCR Policy and Caching

- Status: Accepted
- Date: 2026-03-27
- Deciders: dora56

## Context

OCR-heavy PDF (全ページがスキャン画像) の抽出で、現在の 2 パス OCR 戦略が非効率:

1. **Fast pass** (scale 1.5x, recognition_level=1): 全ページに実行
2. **Accurate pass** (scale 2.0x, recognition_level=0): fast pass で 50 文字未満のページに再実行

455 ページの OCR-heavy PDF では 100% のページが retry され、Vision API を **910 回** 呼び出す (455 × 2 パス)。
fast pass のコスト (~100ms/page × 455 = ~45.5 秒) が完全に無駄になっている。

また、`--force` での再 ingest 時に同一ページの OCR を毎回やり直している。

## Decision

### 1. OCR-heavy 時の accurate-only ポリシー

probe で `extraction_mode == "ocr-heavy"` と判定された書籍には `ocr_policy = "accurate-only"` を自動適用する。
fast pass をスキップし、直接 accurate OCR (scale 2.0x, level 0) を実行する。

### 2. ファイルハッシュベースの OCR 結果キャッシュ

OCR 結果を `~/.refloom/cache/ocr/` にディスクキャッシュする。
キー: `sha256(file_hash:page_num:render_scale:recognition_level)`

### 検討した選択肢

| 選択肢 | 説明 | 判定 |
|---|---|---|
| A) 現状維持 (fast → retry) | 常に 2 パス | 却下 — OCR-heavy で 100% retry |
| **B) probe の mode に基づく policy 切替** | OCR-heavy → accurate-only | **採用** |
| C) confidence-based adaptive | OCR 結果の信頼度で動的判断 | 過剰 — probe の判定で十分 |

### 採用理由

- probe が既に `extraction_mode` を判定しているのでポリシー伝播は自然
- `ocr_policy` フィールドは ExtractPagesRequest に既に存在 (Go/Python 両方)
- キャッシュは `file_hash` ベースでファイル内容変更時に自動無効化
- text-heavy PDF では従来通り `auto` ポリシー (fast → retry) を維持

## Consequences

### Positive

- OCR-heavy PDF の Vision API 呼び出しが 910 → 455 回に半減
- OCR 時間 ~35% 削減 (fast pass コスト排除)
- 再 ingest 時の OCR 時間がほぼゼロに (キャッシュヒット時)

### Negative

- accurate-only は fast pass より遅い (scale 2.0x > 1.5x) が、retry を含めた総コストは減少
- キャッシュディレクトリの管理が必要 (prune 機能は将来追加)

### Neutral

- text-heavy PDF の挙動は変更なし
- キャッシュなしでも動作する (cache miss 時は通常の OCR)
