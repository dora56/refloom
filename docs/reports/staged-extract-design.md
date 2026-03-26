# Staged Extract Design

## Summary

Refloom の extract を single-call worker から staged job へ変更した。Go CLI が `probe`, `extract-pages`, `chunk` を順に呼び出し、中間状態を `~/.refloom/work/<job-id>/` に保存する。

## Worker Protocol

### `probe`

- Input: `path`, `format`
- Output:
  - `book`
  - `chapters`
  - `extraction_mode`
  - `recommended_batch_size`
  - `ocr_candidate_pages_estimate`

### `extract-pages`

- Input:
  - `path`
  - `format`
  - `page_start`
  - `page_end`
  - `ocr_policy`
  - `output_path`
- Output:
  - `pages_written`
  - `stats`
  - `batch_ms`

worker は `output_path` に page JSONL を書く。

### `chunk`

- Input:
  - `format`
  - `pages_path`
  - `chapters_path`
  - `output_path`
  - `options`
- Output:
  - `quality`
  - `chunks_written`
  - `chunk_ms`

worker は `output_path` に chunk JSONL を書く。

## Workdir Layout

```text
~/.refloom/work/<job-id>/
  manifest.json
  chapters.json
  pages/
    000001-000016.jsonl
    000017-000032.jsonl
  pages.all.jsonl
  chunks.jsonl
  metrics.json
```

## Runtime Policy

- PDF probe で first/middle/last のサンプルを見て `ocr-heavy` を判定する
- batch size:
  - `ocr-heavy`: `16`
  - その他: `64`
- timeout:
  - `worker_probe`: `2m`
  - `worker_batch`: `5m`
  - `worker_chunk`: `3m`
- retry:
  - batch ごとに最大 3 回
  - 成功 batch は manifest を見て再実行しない

## Observability

`refloom ingest --profile-json` と validation summary で次を出す。

- `probe_ms`
- `page_extract_ms`
- `chunk_ms`
- `extract_ms`
- `batch_count`
- `failed_batch_count`
- `resumed`
- `job_dir`

## Known Follow-ups

- `~/.refloom/work` の cleanup policy
- interrupted job の resume UX 明確化
- OCR-heavy PDF の batch timeout / retry 指標を benchmark に追加
