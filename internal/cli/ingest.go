package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/dora56/refloom/internal/extraction"
	"github.com/spf13/cobra"
)

var (
	ingestForce         bool
	ingestTags          []string
	ingestProfileJSON   bool
	ingestSkipEmbedding bool
	ingestKeepWork      bool
)

const embeddingBatchSize = 64
const embeddingProgressInterval = 100

type ingestProfile struct {
	Status                  string `json:"status,omitempty"`
	File                    string `json:"file"`
	Format                  string `json:"format,omitempty"`
	Quality                 string `json:"quality,omitempty"`
	Chapters                int    `json:"chapters"`
	Chunks                  int    `json:"chunks"`
	OCRPages                int    `json:"ocr_pages"`
	OCRRetries              int    `json:"ocr_retries,omitempty"`
	OCRMS                   int64  `json:"ocr_ms,omitempty"`
	OCRFastPages            int    `json:"ocr_fast_pages,omitempty"`
	OCRRetryPages           int    `json:"ocr_retry_pages,omitempty"`
	HashMS                  int64  `json:"hash_ms"`
	ProbeMS                 int64  `json:"probe_ms,omitempty"`
	PageExtractMS           int64  `json:"page_extract_ms,omitempty"`
	PageExtractSumMS        int64  `json:"page_extract_sum_ms,omitempty"`
	ChunkMS                 int64  `json:"chunk_ms,omitempty"`
	ExtractMS               int64  `json:"extract_ms"`
	BatchCount              int    `json:"batch_count,omitempty"`
	FailedBatchCount        int    `json:"failed_batch_count,omitempty"`
	ExtractWorkersMode      string `json:"extract_workers_mode,omitempty"`
	ExtractWorkersRequested string `json:"extract_workers_requested,omitempty"`
	ExtractWorkersUsed      int    `json:"extract_workers_used,omitempty"`
	ExtractAutoMaxWorkers   int    `json:"extract_auto_max_workers,omitempty"`
	ExtractAutoEffectiveCap int    `json:"extract_auto_effective_cap,omitempty"`
	ExtractAutoTier         string `json:"extract_auto_tier,omitempty"`
	ExtractAutoCandidates   []int  `json:"extract_auto_candidates,omitempty"`
	ParallelExtractEnabled  bool   `json:"parallel_extract_enabled,omitempty"`
	AutoWorkerReason        string `json:"auto_worker_reason,omitempty"`
	Resumed                 bool   `json:"resumed,omitempty"`
	JobDir                  string `json:"job_dir,omitempty"`
	DBMS                    int64  `json:"db_ms"`
	FTSMS                   int64  `json:"fts_ms"`
	EmbedMS                 int64  `json:"embed_ms"`
	EmbedSkipped            bool   `json:"embed_skipped,omitempty"`
	EmbedBatchSize          int    `json:"embed_batch_size,omitempty"`
	EmbedBatches            int    `json:"embed_batches,omitempty"`
	TotalMS                 int64  `json:"total_ms"`
	EmbedModel              string `json:"embed_model,omitempty"`
	Error                   string `json:"error,omitempty"`
}

type embeddedChunk struct {
	ChunkID int64
	Heading string
	Body    string
}

type insertedChunk struct {
	embeddedChunk
	ChapterID int64
}

type embeddingRunStats struct {
	Fails     int
	Batches   int
	RequestMS int64
	SaveMS    int64
}

var ingestCmd = &cobra.Command{
	Use:   "ingest <path>",
	Short: "Ingest PDF/EPUB books into the local database",
	Args:  cobra.ExactArgs(1),
	RunE:  runIngest,
}

func init() {
	ingestCmd.Flags().BoolVar(&ingestForce, "force", false, "Re-ingest even if book already exists")
	ingestCmd.Flags().StringSliceVar(&ingestTags, "tag", nil, "Add tags to the book")
	ingestCmd.Flags().BoolVar(&ingestProfileJSON, "profile-json", false, "Print one JSON ingest profile per processed book")
	ingestCmd.Flags().BoolVar(&ingestSkipEmbedding, "skip-embedding", false, "Skip embedding generation after extract + DB/FTS")
	ingestCmd.Flags().BoolVar(&ingestKeepWork, "keep-work", false, "Keep work directory after successful ingest")
}

func runIngest(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Ingest)
	defer cancel()

	files, err := findBookFiles(args[0])
	if err != nil {
		return fmt.Errorf("find files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no PDF/EPUB files found at %s", args[0])
	}
	slog.Info("found book files", "count", len(files))

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	workerDir, pythonPath := findWorkerPaths()
	slog.Debug("python worker", "dir", workerDir, "python", pythonPath)
	worker := extraction.NewWorker(pythonPath, workerDir)

	var embedClient *embedding.Client
	if !ingestSkipEmbedding {
		embedClient = embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)
		if err := embedClient.CheckHealth(ctx); err != nil {
			return fmt.Errorf("ollama check: %w", err)
		}
	}

	var failCount int
	for i, file := range files {
		if ingestProfileJSON {
			fmt.Fprintf(os.Stderr, "[%d/%d] Processing %s...\n", i+1, len(files), filepath.Base(file))
		} else {
			fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(files), filepath.Base(file))
		}

		profile, err := ingestFile(ctx, database, worker, embedClient, file)
		if ingestProfileJSON && profile != nil {
			if err := emitIngestProfile(profile); err != nil {
				return err
			}
		}
		if err != nil {
			failCount++
			slog.Error("ingest failed", "file", filepath.Base(file), "error", err)
			continue
		}
	}

	if failCount > 0 {
		return fmt.Errorf("%d of %d files failed to ingest", failCount, len(files))
	}
	return nil
}

func ingestFile(ctx context.Context, database *db.DB, worker *extraction.Worker, embedClient *embedding.Client, filePath string) (_ *ingestProfile, err error) {
	totalStart := time.Now()

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	log := slog.With("path", filepath.Base(absPath))
	profile := &ingestProfile{
		File:   absPath,
		Status: "completed",
	}
	defer func() {
		profile.TotalMS = time.Since(totalStart).Milliseconds()
		if err != nil {
			profile.Status = "failed"
			profile.Error = err.Error()
		}
	}()

	format := detectFormat(absPath)
	if format == "" {
		return profile, fmt.Errorf("unsupported file format")
	}
	profile.Format = format
	profile.EmbedModel = cfg.OllamaEmbedModel

	existing, err := database.GetBookByPath(absPath)
	if err != nil {
		return profile, err
	}
	if existing != nil && !ingestForce {
		log.Info("skipped (already ingested, use --force to re-ingest)")
		profile.Status = "skipped"
		return profile, nil
	}
	if existing != nil && ingestForce {
		log.Info("re-ingesting (--force)", "book_id", existing.BookID)
		if err := database.DeleteBook(existing.BookID); err != nil {
			return profile, fmt.Errorf("delete existing: %w", err)
		}
	}

	hashStart := time.Now()
	hash, err := fileHash(absPath)
	if err != nil {
		return profile, fmt.Errorf("hash: %w", err)
	}
	profile.HashMS = time.Since(hashStart).Milliseconds()

	// Check for duplicate by file hash (same content, different path)
	if existing == nil {
		dupByHash, err := database.GetBookByHash(hash)
		if err == nil && dupByHash != nil && !ingestForce {
			log.Info("skipped (same file already ingested at different path)",
				"existing_path", dupByHash.SourcePath, "book_id", dupByHash.BookID)
			profile.Status = "skipped"
			return profile, nil
		}
	}

	log.Info("extracting", "format", format)
	resp, err := runStagedExtraction(ctx, worker, absPath, format, hash)
	if err != nil {
		return profile, fmt.Errorf("extract: %w", err)
	}
	profile.ProbeMS = resp.ProbeMS
	profile.PageExtractMS = resp.PageExtractMS
	profile.PageExtractSumMS = resp.PageExtractSumMS
	profile.ChunkMS = resp.ChunkMS
	profile.ExtractMS = resp.ProbeMS + resp.PageExtractMS + resp.ChunkMS
	profile.BatchCount = resp.BatchCount
	profile.FailedBatchCount = resp.FailedBatchCount
	profile.ExtractWorkersMode = resp.ExtractWorkersMode
	profile.ExtractWorkersRequested = resp.ExtractWorkersRequested
	profile.ExtractWorkersUsed = resp.ExtractWorkersUsed
	profile.ExtractAutoMaxWorkers = resp.ExtractAutoMaxWorkers
	profile.ExtractAutoEffectiveCap = resp.ExtractAutoEffectiveCap
	profile.ExtractAutoTier = resp.ExtractAutoTier
	profile.ExtractAutoCandidates = append([]int(nil), resp.ExtractAutoCandidates...)
	profile.ParallelExtractEnabled = resp.ExtractWorkersUsed > 1
	profile.AutoWorkerReason = resp.AutoWorkerReason
	profile.Resumed = resp.Resumed
	profile.JobDir = resp.JobDir
	quality := resp.Quality
	if quality == "" {
		quality = "ok"
	}
	profile.Quality = quality
	profile.Chapters = len(resp.Chapters)
	profile.Chunks = len(resp.Chunks)
	profile.OCRPages = resp.Stats.OCRPages
	profile.OCRRetries = resp.Stats.OCRRetries
	profile.OCRMS = resp.Stats.OCRMS
	profile.OCRFastPages = resp.Stats.OCRFastPages
	if profile.OCRFastPages == 0 {
		profile.OCRFastPages = resp.Stats.OCRPages
	}
	profile.OCRRetryPages = resp.Stats.OCRRetryPages
	if profile.OCRRetryPages == 0 {
		profile.OCRRetryPages = resp.Stats.OCRRetries
	}
	if profile.OCRRetries == 0 {
		profile.OCRRetries = profile.OCRRetryPages
	}
	log.Info("extracted",
		"chapters", len(resp.Chapters),
		"chunks", len(resp.Chunks),
		"quality", quality,
		"probe_duration", time.Duration(profile.ProbeMS)*time.Millisecond,
		"page_extract_duration", time.Duration(profile.PageExtractMS)*time.Millisecond,
		"chunk_duration", time.Duration(profile.ChunkMS)*time.Millisecond,
		"duration", time.Duration(profile.ExtractMS)*time.Millisecond,
		"batches", profile.BatchCount,
		"extract_workers_mode", profile.ExtractWorkersMode,
		"extract_workers_requested", profile.ExtractWorkersRequested,
		"extract_workers_used", profile.ExtractWorkersUsed,
		"extract_auto_max_workers", profile.ExtractAutoMaxWorkers,
		"extract_auto_effective_cap", profile.ExtractAutoEffectiveCap,
		"extract_auto_tier", profile.ExtractAutoTier,
		"extract_auto_candidates", profile.ExtractAutoCandidates,
		"auto_worker_reason", profile.AutoWorkerReason,
		"job_dir", profile.JobDir,
	)

	switch quality {
	case "ocr_required":
		fmt.Fprintf(os.Stderr, "  quality: %s — skipping (no extractable text)\n", quality)
		_ = database.LogIngest(0, "failed", fmt.Sprintf("quality=%s: %s", quality, absPath))
		profile.Status = "skipped"
		return profile, nil
	case "extract_failed":
		fmt.Fprintf(os.Stderr, "  quality: %s — skipping\n", quality)
		_ = database.LogIngest(0, "failed", fmt.Sprintf("quality=%s: %s", quality, absPath))
		profile.Status = "skipped"
		return profile, nil
	case "text_corrupt":
		fmt.Fprintf(os.Stderr, "  quality: %s — proceeding with warning (text may contain mojibake)\n", quality)
	}

	tx, err := database.Begin()
	if err != nil {
		return profile, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	tagsJSON := "[]"
	if len(ingestTags) > 0 {
		tagsJSON = `["` + strings.Join(ingestTags, `","`) + `"]`
	}
	dbStart := time.Now()
	bookRes, err := tx.Exec(
		`INSERT INTO book (title, author, format, source_path, file_hash, tags) VALUES (?, ?, ?, ?, ?, ?)`,
		resp.Book.Title, resp.Book.Author, format, absPath, hash, tagsJSON,
	)
	if err != nil {
		return profile, fmt.Errorf("insert book: %w", err)
	}
	bookID, _ := bookRes.LastInsertId()

	chapterIDMap := make(map[int]int64)
	chapterStmt, err := tx.Prepare(
		`INSERT INTO chapter (book_id, title, chapter_order, page_start, page_end) VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return profile, fmt.Errorf("prepare chapter insert: %w", err)
	}
	defer chapterStmt.Close() //nolint:errcheck

	for _, ch := range resp.Chapters {
		var pageStart, pageEnd sql.NullInt64
		if ch.PageStart != nil {
			pageStart = sql.NullInt64{Int64: int64(*ch.PageStart), Valid: true}
		}
		if ch.PageEnd != nil {
			pageEnd = sql.NullInt64{Int64: int64(*ch.PageEnd), Valid: true}
		}
		res, err := chapterStmt.Exec(
			bookID, ch.Title, ch.Order, pageStart, pageEnd,
		)
		if err != nil {
			return profile, fmt.Errorf("insert chapter %d: %w", ch.Order, err)
		}
		chID, _ := res.LastInsertId()
		chapterIDMap[ch.Order] = chID
	}

	chunkStmt, err := tx.Prepare(
		`INSERT INTO chunk (book_id, chapter_id, heading, body, char_count, page_start, page_end, chunk_order, embedding_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return profile, fmt.Errorf("prepare chunk insert: %w", err)
	}
	defer chunkStmt.Close() //nolint:errcheck

	insertedChunks := make([]insertedChunk, 0, len(resp.Chunks))
	for _, ck := range resp.Chunks {
		chapterID, ok := chapterIDMap[ck.ChapterOrder]
		if !ok {
			log.Warn("chunk references unknown chapter", "chapter_order", ck.ChapterOrder)
			continue
		}
		var pageStart, pageEnd sql.NullInt64
		if ck.PageStart != nil {
			pageStart = sql.NullInt64{Int64: int64(*ck.PageStart), Valid: true}
		}
		if ck.PageEnd != nil {
			pageEnd = sql.NullInt64{Int64: int64(*ck.PageEnd), Valid: true}
		}
		res, err := chunkStmt.Exec(
			bookID, chapterID, ck.Heading, ck.Body, ck.CharCount, pageStart, pageEnd, ck.ChunkOrder, "",
		)
		if err != nil {
			return profile, fmt.Errorf("insert chunk: %w", err)
		}
		id, _ := res.LastInsertId()
		insertedChunks = append(insertedChunks, insertedChunk{
			embeddedChunk: embeddedChunk{
				ChunkID: id,
				Heading: ck.Heading,
				Body:    ck.Body,
			},
			ChapterID: chapterID,
		})
	}
	profile.Chunks = len(insertedChunks)

	// Link prev/next within each chapter
	linkNextStmt, err := tx.Prepare(`UPDATE chunk SET next_chunk_id = ? WHERE chunk_id = ?`)
	if err != nil {
		return profile, fmt.Errorf("prepare link next: %w", err)
	}
	defer linkNextStmt.Close() //nolint:errcheck

	linkPrevStmt, err := tx.Prepare(`UPDATE chunk SET prev_chunk_id = ? WHERE chunk_id = ?`)
	if err != nil {
		return profile, fmt.Errorf("prepare link prev: %w", err)
	}
	defer linkPrevStmt.Close() //nolint:errcheck

	prevByChapter := make(map[int64]int64) // chapterID -> last chunk ID
	for _, chunk := range insertedChunks {
		chapterID := chunk.ChapterID
		chunkID := chunk.ChunkID
		if prevID, exists := prevByChapter[chapterID]; exists {
			if _, err := linkNextStmt.Exec(chunkID, prevID); err != nil {
				return profile, fmt.Errorf("link next: %w", err)
			}
			if _, err := linkPrevStmt.Exec(prevID, chunkID); err != nil {
				return profile, fmt.Errorf("link prev: %w", err)
			}
		}
		prevByChapter[chapterID] = chunkID
	}
	profile.DBMS = time.Since(dbStart).Milliseconds()

	ftsStart := time.Now()
	ftsStmt, err := tx.Prepare(`INSERT INTO chunk_fts_seg(rowid, heading, body) VALUES (?, ?, ?)`)
	if err != nil {
		return profile, fmt.Errorf("prepare segmented FTS insert: %w", err)
	}
	defer ftsStmt.Close() //nolint:errcheck

	for _, chunk := range insertedChunks {
		if err := insertSegmentedFTSTx(ftsStmt, chunk.ChunkID, chunk.Heading, chunk.Body); err != nil {
			return profile, fmt.Errorf("insert segmented FTS: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return profile, fmt.Errorf("commit: %w", err)
	}
	profile.FTSMS = time.Since(ftsStart).Milliseconds()
	_ = database.LogIngest(bookID, "chunked", fmt.Sprintf("%d chapters, %d chunks", len(resp.Chapters), len(insertedChunks)))

	if ingestSkipEmbedding {
		applyEmbeddingSkippedProfile(profile)
		_ = database.LogIngest(bookID, "completed", fmt.Sprintf("%d chunks, embedding skipped", len(insertedChunks)))
		if profile.JobDir != "" && !ingestKeepWork {
			if removeErr := os.RemoveAll(profile.JobDir); removeErr != nil {
				slog.Warn("failed to clean up work directory", "dir", profile.JobDir, "error", removeErr)
			}
		}
		log.Info("done",
			"title", resp.Book.Title,
			"chapters", len(resp.Chapters),
			"chunks", len(insertedChunks),
			"embedding_skipped", true,
		)
		return profile, nil
	}

	log.Info("generating embeddings", "chunks", len(insertedChunks))
	embedStart := time.Now()
	batchSize := resolvedEmbeddingBatchSize(cfg.EmbeddingBatchSize)
	profile.EmbedBatchSize = batchSize
	embedInputs := make([]embeddedChunk, 0, len(insertedChunks))
	for _, chunk := range insertedChunks {
		embedInputs = append(embedInputs, chunk.embeddedChunk)
	}
	embedStats, err := saveChunkEmbeddings(ctx, database, embedClient, cfg.OllamaEmbedModel, batchSize, embedInputs, false, log)
	if err != nil {
		return profile, err
	}
	profile.EmbedMS = time.Since(embedStart).Milliseconds()
	profile.EmbedBatches = embedStats.Batches

	if embedStats.Fails > 0 {
		_ = database.LogIngest(bookID, "embedded", fmt.Sprintf("%d/%d succeeded", len(insertedChunks)-embedStats.Fails, len(insertedChunks)))
		if shouldWarnEmbeddingFailures(embedStats.Fails, len(embedInputs)) {
			slog.Warn("embedding failure rate exceeds 50%; book is FTS-searchable but vector search will be degraded",
				"fails", embedStats.Fails, "total", len(embedInputs),
				"ratio", fmt.Sprintf("%.0f%%", float64(embedStats.Fails)/float64(len(embedInputs))*100))
		}
	}
	_ = database.LogIngest(bookID, "completed", fmt.Sprintf("%d chunks, %d embed failures", len(insertedChunks), embedStats.Fails))

	// Clean up work directory after successful ingest
	if profile.JobDir != "" && !ingestKeepWork {
		if removeErr := os.RemoveAll(profile.JobDir); removeErr != nil {
			slog.Warn("failed to clean up work directory", "dir", profile.JobDir, "error", removeErr)
		}
	}

	log.Info("done",
		"title", resp.Book.Title,
		"chapters", len(resp.Chapters),
		"chunks", len(insertedChunks),
		"embed_fails", embedStats.Fails,
		"embed_batch_size", batchSize,
		"embed_batches", embedStats.Batches,
		"embed_request_ms", embedStats.RequestMS,
		"embed_save_ms", embedStats.SaveMS,
		"embed_duration", time.Duration(profile.EmbedMS)*time.Millisecond,
	)
	return profile, nil
}

func emitIngestProfile(profile *ingestProfile) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(profile)
}

func applyEmbeddingSkippedProfile(profile *ingestProfile) {
	profile.EmbedSkipped = true
	profile.EmbedMS = 0
	profile.EmbedBatches = 0
}

func findBookFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var files []string
	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(strings.ToLower(p), ".pdf") || strings.HasSuffix(strings.ToLower(p), ".epub")) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

func detectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".epub":
		return "epub"
	default:
		return ""
	}
}

func saveChunkEmbeddings(
	ctx context.Context,
	database *db.DB,
	embedClient *embedding.Client,
	model string,
	batchSize int,
	chunks []embeddedChunk,
	replaceExisting bool,
	log *slog.Logger,
) (embeddingRunStats, error) {
	stats := embeddingRunStats{}
	effectiveBatchSize := resolvedEmbeddingBatchSize(batchSize)

	for start := 0; start < len(chunks); start += effectiveBatchSize {
		end := min(start+effectiveBatchSize, len(chunks))

		batch := chunks[start:end]
		processed := start + len(batch)
		stats.Batches++
		if shouldLogEmbeddingProgress(start, processed) {
			log.Info("embedding progress", "done", processed, "total", len(chunks))
		}

		texts := make([]string, 0, len(batch))
		for _, chunk := range batch {
			texts = append(texts, chunk.Body)
		}

		requestStart := time.Now()
		embeddings, err := embedClient.EmbedBatch(ctx, texts)
		stats.RequestMS += time.Since(requestStart).Milliseconds()
		if err != nil || len(embeddings) != len(batch) {
			if err == nil {
				err = fmt.Errorf("embedding batch size mismatch: got %d embeddings for %d chunks", len(embeddings), len(batch))
			}
			log.Warn("embedding batch failed; falling back to single chunk requests", "batch_start", start, "batch_size", len(batch), "error", err)
			for _, chunk := range batch {
				singleRequestStart := time.Now()
				emb, singleErr := embedClient.Embed(ctx, chunk.Body)
				stats.RequestMS += time.Since(singleRequestStart).Milliseconds()
				if singleErr != nil {
					log.Warn("embedding failed", "chunk_id", chunk.ChunkID, "error", singleErr)
					stats.Fails++
					continue
				}
				saveStart := time.Now()
				if singleErr := saveEmbeddingBatch(database, model, []embeddedChunk{chunk}, [][]float32{emb}, replaceExisting); singleErr != nil {
					stats.SaveMS += time.Since(saveStart).Milliseconds()
					log.Warn("save embedding failed", "chunk_id", chunk.ChunkID, "error", singleErr)
					stats.Fails++
					continue
				}
				stats.SaveMS += time.Since(saveStart).Milliseconds()
			}
			continue
		}

		saveStart := time.Now()
		if err := saveEmbeddingBatch(database, model, batch, embeddings, replaceExisting); err != nil {
			stats.SaveMS += time.Since(saveStart).Milliseconds()
			log.Warn("save embedding batch failed; retrying chunk by chunk", "batch_start", start, "batch_size", len(batch), "error", err)
			for i, chunk := range batch {
				singleSaveStart := time.Now()
				if singleErr := saveEmbeddingBatch(database, model, []embeddedChunk{chunk}, [][]float32{embeddings[i]}, replaceExisting); singleErr != nil {
					stats.SaveMS += time.Since(singleSaveStart).Milliseconds()
					log.Warn("save embedding failed", "chunk_id", chunk.ChunkID, "error", singleErr)
					stats.Fails++
					continue
				}
				stats.SaveMS += time.Since(singleSaveStart).Milliseconds()
			}
			continue
		}
		stats.SaveMS += time.Since(saveStart).Milliseconds()
	}

	return stats, nil
}

func shouldLogEmbeddingProgress(previous, current int) bool {
	if current <= previous || current < embeddingProgressInterval {
		return false
	}

	return previous/embeddingProgressInterval != current/embeddingProgressInterval
}

func shouldWarnEmbeddingFailures(fails, total int) bool {
	if total <= 0 {
		return false
	}
	return float64(fails)/float64(total) > 0.5
}

func resolvedEmbeddingBatchSize(configured int) int {
	if configured > 0 {
		return configured
	}
	return embeddingBatchSize
}

func saveEmbeddingBatch(
	database *db.DB,
	model string,
	chunks []embeddedChunk,
	embeddings [][]float32,
	replaceExisting bool,
) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin embedding tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	chunkIDs := make([]int64, 0, len(chunks))
	for _, chunk := range chunks {
		chunkIDs = append(chunkIDs, chunk.ChunkID)
	}
	if err := db.SaveEmbeddingBatchTx(tx, model, chunkIDs, embeddings, replaceExisting); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit embedding tx: %w", err)
	}
	return nil
}

func insertSegmentedFTSTx(stmt *sql.Stmt, chunkID int64, heading, body string) error {
	segHeading := db.SegmentText(heading)
	segBody := db.SegmentText(body)
	if _, err := stmt.Exec(chunkID, segHeading, segBody); err != nil {
		return fmt.Errorf("insert segmented FTS row: %w", err)
	}
	return nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func findWorkerPaths() (workerDir, pythonPath string) {
	if cfg.PythonWorkerDir != "" {
		return resolveConfiguredWorkerDir(cfg.PythonWorkerDir)
	}

	if envDir := os.Getenv("REFLOOM_WORKER_DIR"); envDir != "" {
		return resolveConfiguredWorkerDir(envDir)
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		filepath.Join(exeDir, "..", "python"),
		filepath.Join(exeDir, "python"),
		"python",
		"./python",
		filepath.Join(os.Getenv("HOME"), ".refloom", "python"),
	}

	for _, dir := range candidates {
		absDir, _ := filepath.Abs(dir)
		venvPython := filepath.Join(absDir, "refloom_worker", ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil { //nolint:gosec
			slog.Debug("found python worker", "dir", absDir) //nolint:gosec
			return absDir, venvPython
		}
	}

	slog.Warn("python worker venv not found, falling back to system python")
	return "python", "python3"
}

func resolveConfiguredWorkerDir(configured string) (workerDir, pythonPath string) {
	type candidate struct {
		workerDir  string
		packageDir string
		pythonPath string
	}

	configured = filepath.Clean(configured)
	candidates := []candidate{{
		workerDir:  configured,
		packageDir: filepath.Join(configured, "refloom_worker"),
	}}
	if filepath.Base(configured) == "refloom_worker" {
		candidates = append([]candidate{{
			workerDir:  filepath.Dir(configured),
			packageDir: configured,
		}}, candidates...)
	}
	//nolint:gosec // configured is a trusted local path from user config/env; we only probe for worker layout.
	if info, err := os.Stat(configured); err == nil && !info.IsDir() {
		packageDir := filepath.Dir(filepath.Dir(filepath.Dir(configured)))
		workerDir := packageDir
		if filepath.Base(packageDir) == "refloom_worker" {
			workerDir = filepath.Dir(packageDir)
		}
		candidates = append([]candidate{{
			workerDir:  workerDir,
			packageDir: packageDir,
			pythonPath: configured,
		}}, candidates...)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate.packageDir, "main.py")); err != nil {
			continue
		}
		if candidate.pythonPath != "" {
			return candidate.workerDir, candidate.pythonPath
		}
		venvPython := filepath.Join(candidate.packageDir, ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil {
			return candidate.workerDir, venvPython
		}
		slog.Warn("configured worker dir has no venv", "dir", candidate.workerDir)
		return candidate.workerDir, "python3"
	}

	slog.Warn("configured worker dir has no worker package")
	return configured, "python3"
}
