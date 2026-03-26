package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dora56/refloom/internal/config"
	"github.com/dora56/refloom/internal/extraction"
)

const (
	defaultTextBatchSize = 64
	defaultOCRBatchSize  = 16
	maxBatchAttempts     = 3
)

type extractJobManifest struct {
	JobID                   string                   `json:"job_id"`
	SourcePath              string                   `json:"source_path"`
	Format                  string                   `json:"format"`
	Status                  string                   `json:"status"`
	PageCount               int                      `json:"page_count"`
	BatchSize               int                      `json:"batch_size"`
	ExtractionMode          string                   `json:"extraction_mode,omitempty"`
	Completed               []extractCompletedBatch  `json:"completed_batches"`
	Failed                  []extractFailedBatch     `json:"failed_batches,omitempty"`
	ProbeMS                 int64                    `json:"probe_ms"`
	PageExtractMS           int64                    `json:"page_extract_ms"`
	PageExtractSumMS        int64                    `json:"page_extract_sum_ms,omitempty"`
	OCRMS                   int64                    `json:"ocr_ms"`
	ChunkMS                 int64                    `json:"chunk_ms"`
	TotalMS                 int64                    `json:"total_ms"`
	ExtractWorkersMode      string                   `json:"extract_workers_mode,omitempty"`
	ExtractWorkersRequested string                   `json:"extract_workers_requested,omitempty"`
	ExtractWorkersUsed      int                      `json:"extract_workers_used,omitempty"`
	ExtractAutoMaxWorkers   int                      `json:"extract_auto_max_workers,omitempty"`
	ExtractAutoEffectiveCap int                      `json:"extract_auto_effective_cap,omitempty"`
	ExtractAutoTier         string                   `json:"extract_auto_tier,omitempty"`
	ExtractAutoCandidates   []int                    `json:"extract_auto_candidates,omitempty"`
	AutoWorkerReason        string                   `json:"auto_worker_reason,omitempty"`
	ChaptersPath            string                   `json:"chapters_path,omitempty"`
	PagesPath               string                   `json:"pages_path,omitempty"`
	ChunksPath              string                   `json:"chunks_path,omitempty"`
	Book                    extraction.BookInfo      `json:"book"`
	Chapters                []extraction.ChapterInfo `json:"chapters,omitempty"`
}

type extractCompletedBatch struct {
	PageStart     int              `json:"page_start"`
	PageEnd       int              `json:"page_end"`
	OutputPath    string           `json:"output_path"`
	PagesWritten  int              `json:"pages_written"`
	OCRPages      int              `json:"ocr_pages"`
	OCRRetryPages int              `json:"ocr_retry_pages"`
	OCRMS         int64            `json:"ocr_ms"`
	BatchMS       int64            `json:"batch_ms"`
	Stats         extraction.Stats `json:"stats"`
}

type extractFailedBatch struct {
	PageStart int    `json:"page_start"`
	PageEnd   int    `json:"page_end"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
}

type stagedExtractResult struct {
	Book                    extraction.BookInfo
	Chapters                []extraction.ChapterInfo
	Chunks                  []extraction.ChunkInfo
	Quality                 string
	Stats                   extraction.Stats
	ProbeMS                 int64
	PageExtractMS           int64
	PageExtractSumMS        int64
	ChunkMS                 int64
	BatchCount              int
	FailedBatchCount        int
	Resumed                 bool
	JobDir                  string
	ExtractWorkersMode      string
	ExtractWorkersRequested string
	ExtractWorkersUsed      int
	ExtractAutoMaxWorkers   int
	ExtractAutoEffectiveCap int
	ExtractAutoTier         string
	ExtractAutoCandidates   []int
	AutoWorkerReason        string
}

type pageBatchRange struct {
	PageStart int
	PageEnd   int
}

type extractPagesResult struct {
	batch     pageBatchRange
	completed *extractCompletedBatch
	attempts  int
	err       error
}

func runStagedExtraction(ctx context.Context, worker *extraction.Worker, absPath, format, jobID string) (*stagedExtractResult, error) {
	start := time.Now()
	jobDir, manifest, resumed, err := prepareExtractJob(absPath, format, jobID)
	if err != nil {
		return nil, err
	}

	probeResp, err := probeAndPrepareManifest(ctx, worker, absPath, format, jobDir, manifest)
	if err != nil {
		return nil, err
	}

	ranges := buildPageBatchRanges(manifest.PageCount, manifest.BatchSize)
	pageExtractStart := time.Now()
	workerPlan, err := extractPageBatches(ctx, worker, absPath, format, jobDir, manifest, ranges, cfg.ExtractBatchWorkers, cfg.ExtractAutoMaxWorkers)
	if err != nil {
		manifest.Status = "failed"
		manifest.TotalMS = time.Since(start).Milliseconds()
		_ = saveExtractManifest(jobDir, manifest)
		return nil, err
	}
	finalizePageExtractMetrics(manifest, time.Since(pageExtractStart))

	chunkResp, chunks, err := finalizeExtractJob(ctx, worker, format, jobDir, manifest, start, workerPlan)
	if err != nil {
		manifest.Status = "failed"
		manifest.TotalMS = time.Since(start).Milliseconds()
		_ = saveExtractManifest(jobDir, manifest)
		return nil, err
	}

	stats := extraction.Stats{}
	for _, batch := range manifest.Completed {
		stats.OCRPages += batch.Stats.OCRPages
		stats.OCRRetries += batch.Stats.OCRRetries
		stats.OCRMS += batch.Stats.OCRMS
		stats.OCRFastPages += batch.Stats.OCRFastPages
		stats.OCRRetryPages += batch.Stats.OCRRetryPages
		stats.OCRFastMS += batch.Stats.OCRFastMS
		stats.OCRRetryMS += batch.Stats.OCRRetryMS
	}

	return &stagedExtractResult{
		Book:                    probeResp.Book,
		Chapters:                probeResp.Chapters,
		Chunks:                  chunks,
		Quality:                 chunkResp.Quality,
		Stats:                   stats,
		ProbeMS:                 manifest.ProbeMS,
		PageExtractMS:           manifest.PageExtractMS,
		PageExtractSumMS:        manifest.PageExtractSumMS,
		ChunkMS:                 manifest.ChunkMS,
		BatchCount:              len(manifest.Completed),
		FailedBatchCount:        len(manifest.Failed),
		Resumed:                 resumed,
		JobDir:                  jobDir,
		ExtractWorkersMode:      workerPlan.Mode,
		ExtractWorkersRequested: workerPlan.Requested,
		ExtractWorkersUsed:      workerPlan.Used,
		ExtractAutoMaxWorkers:   workerPlan.ConfiguredAutoCap,
		ExtractAutoEffectiveCap: workerPlan.EffectiveAutoCap,
		ExtractAutoTier:         workerPlan.AutoTier,
		ExtractAutoCandidates:   append([]int(nil), workerPlan.AutoCandidates...),
		AutoWorkerReason:        workerPlan.Reason,
	}, nil
}

func probeAndPrepareManifest(ctx context.Context, worker *extraction.Worker, absPath, format, jobDir string, manifest *extractJobManifest) (*extraction.ProbeResponse, error) {
	probeCtx, probeCancel := context.WithTimeout(ctx, cfg.Timeouts.WorkerProbe)
	probeStart := time.Now()
	probeResp, err := worker.Probe(probeCtx, absPath, format)
	probeCancel()
	if err != nil {
		return nil, fmt.Errorf("probe: %w", err)
	}
	manifest.ProbeMS = time.Since(probeStart).Milliseconds()
	manifest.Book = probeResp.Book
	manifest.Chapters = probeResp.Chapters
	manifest.PageCount = probeResp.Book.PageCount
	manifest.ExtractionMode = probeResp.ExtractionMode
	manifest.BatchSize = resolvedExtractBatchSize(probeResp.ExtractionMode, probeResp.RecommendedBatchSize)
	manifest.ChaptersPath = filepath.Join(jobDir, "chapters.json")
	manifest.PagesPath = filepath.Join(jobDir, "pages.all.jsonl")
	manifest.ChunksPath = filepath.Join(jobDir, "chunks.jsonl")
	manifest.Status = "extracting_pages"
	if err := writeJSONFile(manifest.ChaptersPath, manifest.Chapters); err != nil {
		return nil, fmt.Errorf("write chapters: %w", err)
	}
	if err := saveExtractManifest(jobDir, manifest); err != nil {
		return nil, err
	}
	return probeResp, nil
}

func extractPageBatches(
	ctx context.Context,
	worker *extraction.Worker,
	absPath, format, jobDir string,
	manifest *extractJobManifest,
	ranges []pageBatchRange,
	setting config.ExtractBatchWorkersSetting,
	autoMaxWorkers int,
) (extractWorkerPlan, error) {
	pending := pendingPageBatchRanges(manifest, ranges)
	plan := resolveExtractWorkerPlan(format, manifest.ExtractionMode, setting, autoMaxWorkers, 0, hostExtractObservation{}, 0)
	plan.Reason = "no pending extract batches"
	if len(pending) == 0 {
		applyExtractWorkerPlan(manifest, plan)
		if err := saveExtractManifest(jobDir, manifest); err != nil {
			return plan, err
		}
		return plan, nil
	}

	host := hostExtractObservation{}
	warmupAvgMS := int64(0)
	if setting.Auto && isParallelExtractCandidate(format, manifest.ExtractionMode) {
		host = observeHostExtractCapacity()
		warmup := pending[:min(autoWarmupBatchCount, len(pending))]
		if len(warmup) > 0 {
			var warmupSum int64
			if err := extractBatchesSequential(ctx, worker, absPath, format, jobDir, manifest, warmup); err != nil {
				return plan, err
			}
			for _, batch := range warmup {
				completed := manifest.findCompletedBatch(batch)
				if completed == nil {
					return plan, fmt.Errorf("warm-up batch %d-%d did not complete", batch.PageStart, batch.PageEnd)
				}
				warmupSum += completed.BatchMS
			}
			warmupAvgMS = warmupSum / int64(len(warmup))
			pending = pendingPageBatchRanges(manifest, ranges)
		}
	}

	plan = resolveExtractWorkerPlan(format, manifest.ExtractionMode, setting, autoMaxWorkers, len(pending), host, warmupAvgMS)
	applyExtractWorkerPlan(manifest, plan)
	if err := saveExtractManifest(jobDir, manifest); err != nil {
		return plan, err
	}
	if len(pending) == 0 {
		return plan, nil
	}
	if plan.Used <= 1 || len(pending) == 1 {
		return plan, extractBatchesSequential(ctx, worker, absPath, format, jobDir, manifest, pending)
	}
	return plan, extractBatchesConcurrent(ctx, worker, absPath, format, jobDir, manifest, pending, plan.Used)
}

func extractBatchesSequential(ctx context.Context, worker *extraction.Worker, absPath, format, jobDir string, manifest *extractJobManifest, pending []pageBatchRange) error {
	for _, batch := range pending {
		result := runExtractBatch(ctx, worker, absPath, format, jobDir, batch)
		if err := applyExtractBatchResult(jobDir, manifest, result); err != nil {
			return err
		}
		if result.err != nil {
			return fmt.Errorf("extract pages %d-%d: %w", batch.PageStart, batch.PageEnd, result.err)
		}
	}
	return nil
}

func extractBatchesConcurrent(ctx context.Context, worker *extraction.Worker, absPath, format, jobDir string, manifest *extractJobManifest, pending []pageBatchRange, workers int) error {
	extractCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	batchCh := make(chan pageBatchRange)
	resultCh := make(chan extractPagesResult)
	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for batch := range batchCh {
				result := runExtractBatch(extractCtx, worker, absPath, format, jobDir, batch)
				select {
				case resultCh <- result:
				case <-extractCtx.Done():
					return
				}
				if result.err != nil {
					return
				}
			}
		})
	}

	go func() {
		defer close(resultCh)
		wg.Wait()
	}()

	go func() {
		defer close(batchCh)
		for _, batch := range pending {
			select {
			case <-extractCtx.Done():
				return
			case batchCh <- batch:
			}
		}
	}()

	var firstErr error
	for result := range resultCh {
		if err := applyExtractBatchResult(jobDir, manifest, result); err != nil && firstErr == nil {
			firstErr = err
			cancel()
			continue
		}
		if result.err != nil && !errors.Is(result.err, context.Canceled) && firstErr == nil {
			firstErr = fmt.Errorf("extract pages %d-%d: %w", result.batch.PageStart, result.batch.PageEnd, result.err)
			cancel()
		}
	}
	return firstErr
}

func runExtractBatch(ctx context.Context, worker *extraction.Worker, absPath, format, jobDir string, batch pageBatchRange) extractPagesResult {
	result := extractPagesResult{batch: batch}
	for attempt := 1; attempt <= maxBatchAttempts; attempt++ {
		if ctx.Err() != nil {
			result.attempts = attempt - 1
			result.err = ctx.Err()
			return result
		}

		outputPath := batchOutputPath(jobDir, batch)
		batchCtx, batchCancel := context.WithTimeout(ctx, cfg.Timeouts.WorkerBatch)
		resp, err := worker.ExtractPages(batchCtx, extraction.ExtractPagesRequest{
			Path:       absPath,
			Format:     format,
			PageStart:  batch.PageStart,
			PageEnd:    batch.PageEnd,
			OCRPolicy:  "auto",
			OutputPath: outputPath,
		})
		batchCancel()
		result.attempts = attempt
		if err != nil {
			result.err = err
			continue
		}
		result.completed = &extractCompletedBatch{
			PageStart:     batch.PageStart,
			PageEnd:       batch.PageEnd,
			OutputPath:    outputPath,
			PagesWritten:  resp.PagesWritten,
			OCRPages:      resp.Stats.OCRPages,
			OCRRetryPages: resp.Stats.OCRRetryPages,
			OCRMS:         resp.Stats.OCRMS,
			BatchMS:       resp.BatchMS,
			Stats:         resp.Stats,
		}
		result.err = nil
		return result
	}
	return result
}

func applyExtractBatchResult(jobDir string, manifest *extractJobManifest, result extractPagesResult) error {
	if result.err != nil {
		if errors.Is(result.err, context.Canceled) {
			return nil
		}
		manifest.recordFailedBatch(result.batch, result.attempts, result.err)
		return saveExtractManifest(jobDir, manifest)
	}
	if result.completed == nil {
		return nil
	}
	manifest.upsertCompletedBatch(*result.completed)
	manifest.PageExtractSumMS += result.completed.BatchMS
	manifest.OCRMS += result.completed.Stats.OCRMS
	manifest.clearFailedBatch(result.batch)
	return saveExtractManifest(jobDir, manifest)
}

func finalizePageExtractMetrics(manifest *extractJobManifest, elapsed time.Duration) {
	manifest.PageExtractMS = elapsed.Milliseconds()
}

func finalizeExtractJob(ctx context.Context, worker *extraction.Worker, format, jobDir string, manifest *extractJobManifest, start time.Time, workerPlan extractWorkerPlan) (*extraction.ChunkResponse, []extraction.ChunkInfo, error) {
	if err := mergePageBatches(manifest.PagesPath, manifest.Completed); err != nil {
		return nil, nil, fmt.Errorf("merge page batches: %w", err)
	}

	manifest.Status = "chunking"
	if err := saveExtractManifest(jobDir, manifest); err != nil {
		return nil, nil, err
	}

	chunkCtx, chunkCancel := context.WithTimeout(ctx, cfg.Timeouts.WorkerChunk)
	chunkResp, err := worker.Chunk(chunkCtx, extraction.ChunkRequest{
		Format:       format,
		PagesPath:    manifest.PagesPath,
		ChaptersPath: manifest.ChaptersPath,
		OutputPath:   manifest.ChunksPath,
		Options: extraction.Options{
			ChunkSize:    cfg.ChunkSize,
			ChunkOverlap: cfg.ChunkOverlap,
		},
	})
	chunkCancel()
	if err != nil {
		return nil, nil, fmt.Errorf("chunk: %w", err)
	}
	manifest.ChunkMS = chunkResp.ChunkMS
	manifest.Status = "completed"
	manifest.TotalMS = time.Since(start).Milliseconds()
	if err := saveExtractManifest(jobDir, manifest); err != nil {
		return nil, nil, err
	}
	if err := writeJSONFile(filepath.Join(jobDir, "metrics.json"), map[string]any{
		"probe_ms":                   manifest.ProbeMS,
		"page_extract_ms":            manifest.PageExtractMS,
		"page_extract_sum_ms":        manifest.PageExtractSumMS,
		"ocr_ms":                     manifest.OCRMS,
		"chunk_ms":                   manifest.ChunkMS,
		"total_ms":                   manifest.TotalMS,
		"batch_count":                len(manifest.Completed),
		"failed_batch_count":         len(manifest.Failed),
		"extract_workers_mode":       workerPlan.Mode,
		"extract_workers_requested":  workerPlan.Requested,
		"extract_workers_used":       workerPlan.Used,
		"extract_auto_max_workers":   workerPlan.ConfiguredAutoCap,
		"extract_auto_effective_cap": workerPlan.EffectiveAutoCap,
		"extract_auto_tier":          workerPlan.AutoTier,
		"extract_auto_candidates":    workerPlan.AutoCandidates,
		"parallel_extract_enabled":   workerPlan.Used > 1,
		"auto_worker_reason":         workerPlan.Reason,
	}); err != nil {
		return nil, nil, fmt.Errorf("write metrics: %w", err)
	}

	chunks, err := loadChunksJSONL(manifest.ChunksPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load chunks: %w", err)
	}
	return chunkResp, chunks, nil
}

func applyExtractWorkerPlan(manifest *extractJobManifest, plan extractWorkerPlan) {
	manifest.ExtractWorkersMode = plan.Mode
	manifest.ExtractWorkersRequested = plan.Requested
	manifest.ExtractWorkersUsed = plan.Used
	manifest.ExtractAutoMaxWorkers = plan.ConfiguredAutoCap
	manifest.ExtractAutoEffectiveCap = plan.EffectiveAutoCap
	manifest.ExtractAutoTier = plan.AutoTier
	manifest.ExtractAutoCandidates = append([]int(nil), plan.AutoCandidates...)
	manifest.AutoWorkerReason = plan.Reason
}

func prepareExtractJob(absPath, format, jobID string) (string, *extractJobManifest, bool, error) {
	workRoot, err := extractWorkRoot()
	if err != nil {
		return "", nil, false, err
	}
	jobDir := filepath.Join(workRoot, jobID)
	if err := ensureExtractJobDirs(jobDir); err != nil {
		return "", nil, false, err
	}

	if _, err := os.Stat(filepath.Join(jobDir, "manifest.json")); err == nil {
		manifest, loadErr := loadExtractManifest(jobDir)
		if loadErr != nil {
			return "", nil, false, loadErr
		}
		if manifest.SourcePath == absPath && manifest.Format == format && manifest.Status != "completed" {
			return jobDir, manifest, len(manifest.Completed) > 0, nil
		}
		if err := resetExtractJobDir(jobDir); err != nil {
			return "", nil, false, err
		}
	}

	manifest := &extractJobManifest{
		JobID:      jobID,
		SourcePath: absPath,
		Format:     format,
		Status:     "created",
	}
	return jobDir, manifest, false, nil
}

func ensureExtractJobDirs(jobDir string) error {
	if err := os.MkdirAll(filepath.Join(jobDir, "pages"), 0o750); err != nil {
		return fmt.Errorf("create job dir: %w", err)
	}
	return nil
}

func resetExtractJobDir(jobDir string) error {
	if err := os.RemoveAll(jobDir); err != nil {
		return fmt.Errorf("reset job dir: %w", err)
	}
	if err := ensureExtractJobDirs(jobDir); err != nil {
		return err
	}
	return nil
}

func extractWorkRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(homeDir, ".refloom", "work"), nil
}

func resolvedExtractBatchSize(mode string, recommended int) int {
	if recommended > 0 {
		return recommended
	}
	if strings.EqualFold(mode, "ocr-heavy") {
		return defaultOCRBatchSize
	}
	return defaultTextBatchSize
}

func buildPageBatchRanges(pageCount, batchSize int) []pageBatchRange {
	if pageCount <= 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultTextBatchSize
	}
	ranges := make([]pageBatchRange, 0, (pageCount+batchSize-1)/batchSize)
	for start := 1; start <= pageCount; start += batchSize {
		end := min(start+batchSize-1, pageCount)
		ranges = append(ranges, pageBatchRange{PageStart: start, PageEnd: end})
	}
	return ranges
}

func pendingPageBatchRanges(manifest *extractJobManifest, ranges []pageBatchRange) []pageBatchRange {
	pending := make([]pageBatchRange, 0, len(ranges))
	for _, batch := range ranges {
		if manifest.findCompletedBatch(batch) == nil {
			pending = append(pending, batch)
		}
	}
	return pending
}

func batchOutputPath(jobDir string, batch pageBatchRange) string {
	return filepath.Join(jobDir, "pages", fmt.Sprintf("%06d-%06d.jsonl", batch.PageStart, batch.PageEnd))
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write json file: %w", err)
	}
	return nil
}

func loadExtractManifest(jobDir string) (*extractJobManifest, error) {
	data, err := os.ReadFile(filepath.Join(jobDir, "manifest.json")) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest extractJobManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &manifest, nil
}

func saveExtractManifest(jobDir string, manifest *extractJobManifest) error {
	return writeJSONFile(filepath.Join(jobDir, "manifest.json"), manifest)
}

func mergePageBatches(outputPath string, batches []extractCompletedBatch) error {
	slices.SortFunc(batches, func(a, b extractCompletedBatch) int {
		return a.PageStart - b.PageStart
	})
	outFile, err := os.Create(outputPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("create merged pages file: %w", err)
	}
	defer outFile.Close() //nolint:errcheck

	writer := bufio.NewWriter(outFile)

	for _, batch := range batches {
		inFile, err := os.Open(batch.OutputPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("open batch file: %w", err)
		}
		if err := func() error {
			defer inFile.Close() //nolint:errcheck

			scanner := bufio.NewScanner(inFile)
			scanner.Buffer(make([]byte, 1024), 16*1024*1024)
			for scanner.Scan() {
				if _, err := writer.WriteString(scanner.Text() + "\n"); err != nil {
					return fmt.Errorf("write merged pages: %w", err)
				}
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("scan batch file: %w", err)
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush merged pages: %w", err)
	}
	return nil
}

func loadChunksJSONL(path string) ([]extraction.ChunkInfo, error) {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("open chunk jsonl: %w", err)
	}
	defer file.Close() //nolint:errcheck

	var chunks []extraction.ChunkInfo
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk extraction.ChunkInfo
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return nil, fmt.Errorf("parse chunk jsonl: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan chunk jsonl: %w", err)
	}
	return chunks, nil
}

func (m *extractJobManifest) findCompletedBatch(batch pageBatchRange) *extractCompletedBatch {
	for i := range m.Completed {
		if m.Completed[i].PageStart == batch.PageStart && m.Completed[i].PageEnd == batch.PageEnd {
			return &m.Completed[i]
		}
	}
	return nil
}

func (m *extractJobManifest) upsertCompletedBatch(batch extractCompletedBatch) {
	for i := range m.Completed {
		if m.Completed[i].PageStart == batch.PageStart && m.Completed[i].PageEnd == batch.PageEnd {
			m.Completed[i] = batch
			return
		}
	}
	m.Completed = append(m.Completed, batch)
}

func (m *extractJobManifest) recordFailedBatch(batch pageBatchRange, attempts int, err error) {
	for i := range m.Failed {
		if m.Failed[i].PageStart == batch.PageStart && m.Failed[i].PageEnd == batch.PageEnd {
			m.Failed[i].Attempts = attempts
			m.Failed[i].LastError = err.Error()
			return
		}
	}
	m.Failed = append(m.Failed, extractFailedBatch{
		PageStart: batch.PageStart,
		PageEnd:   batch.PageEnd,
		Attempts:  attempts,
		LastError: err.Error(),
	})
}

func (m *extractJobManifest) clearFailedBatch(batch pageBatchRange) {
	filtered := m.Failed[:0]
	for _, failed := range m.Failed {
		if failed.PageStart == batch.PageStart && failed.PageEnd == batch.PageEnd {
			continue
		}
		filtered = append(filtered, failed)
	}
	m.Failed = filtered
}
