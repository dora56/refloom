# Changelog

All notable changes to Refloom.

## [Unreleased]

## [0.2.0] - 2026-03-27

### Added

- Add persistent Python worker pool for subprocess reuse (ADR-0007)
- Add OCR accurate-only policy for OCR-heavy books (ADR-0008)
- Add OCR result caching at ~/.refloom/cache/ocr/ (ADR-0008)
- Add parallel embedding batch workers (2-4 goroutines) (ADR-0009)
- Add staged extraction with probe → extract-pages → chunk pipeline (ADR-0001)
- Add auto extract worker scaling based on Apple Silicon tier detection
- Add delete, config, doctor, export, and update-check commands
- Add `--keep-work` flag and work directory auto-cleanup
- Add partial failure reporting for multi-file ingest
- Add Extractor interface for testable extraction
- Add golangci-lint, ruff, pyright for code quality
- Add CI workflow (GitHub Actions)
- Add Apple Vision Framework OCR for scanned PDF pages
- Add keyword alias expansion and 100-book scale estimation
- Add morphological FTS using kagome for Japanese tokenization
- Add explicit prompt budget enforcement for LLM context
- Add extraction quality classification and text corruption detection
- Add comparison intent detection and book diversification
- Add automated validation pipeline and Codex comparison report
- Add EPUB text cleaning to remove layout artifacts
- Add version command and distribution packaging
- Add DB migration versioning and file-hash deduplication
- Add embedding retry with exponential backoff and ingest_log tracking
- Add PoC evaluation document
- Add Claude Code CLI as default LLM provider
- Add centralized configuration with YAML and env var support
- Add inspect and reindex commands
- Add answer generation with Claude API and citation control
- Add hybrid search engine with FTS5, vector, and RRF fusion
- Add ingest pipeline: Python extraction, Ollama embeddings, SQLite storage
- Add SQLite database layer with FTS5 and sqlite-vec
- Add Python worker for PDF/EPUB extraction and chunking
- Add project scaffolding: Go module, CLI skeleton, Makefile, Python requirements

### Changed

- Refactor ingestFile into smaller functions (338→129 lines)
- Fix embedding retry panic when MaxRetries exceeds delays array length
- Fix tags JSON construction to use json.Marshal for proper escaping
- Fix mergePageBatches flush and file permissions (0o600)
- Fix manifest atomic writes (temp + rename pattern)
- Lower auto worker critical memory threshold from 512 MiB to 256 MiB
- Clean up PoC: remove worktrees, branches, organize docs
- Update PoC evaluation and add final completion report
