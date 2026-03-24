package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
	ingestForce bool
	ingestTags  []string
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <path>",
	Short: "Ingest PDF/EPUB books into the local database",
	Args:  cobra.ExactArgs(1),
	RunE:  runIngest,
}

func init() {
	ingestCmd.Flags().BoolVar(&ingestForce, "force", false, "Re-ingest even if book already exists")
	ingestCmd.Flags().StringSliceVar(&ingestTags, "tag", nil, "Add tags to the book")
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
	defer database.Close()

	workerDir, pythonPath := findWorkerPaths()
	slog.Debug("python worker", "dir", workerDir, "python", pythonPath)
	worker := extraction.NewWorker(pythonPath, workerDir)

	embedClient := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)
	if err := embedClient.CheckHealth(ctx); err != nil {
		return fmt.Errorf("ollama check: %w", err)
	}

	for i, file := range files {
		fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(files), filepath.Base(file))
		if err := ingestFile(ctx, database, worker, embedClient, file); err != nil {
			slog.Error("ingest failed", "file", filepath.Base(file), "error", err)
			continue
		}
	}

	return nil
}

func ingestFile(ctx context.Context, database *db.DB, worker *extraction.Worker, embedClient *embedding.Client, filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	log := slog.With("path", filepath.Base(absPath))

	existing, err := database.GetBookByPath(absPath)
	if err != nil {
		return err
	}
	if existing != nil && !ingestForce {
		log.Info("skipped (already ingested, use --force to re-ingest)")
		return nil
	}
	if existing != nil && ingestForce {
		log.Info("re-ingesting (--force)", "book_id", existing.BookID)
		if err := database.DeleteBook(existing.BookID); err != nil {
			return fmt.Errorf("delete existing: %w", err)
		}
	}

	hash, err := fileHash(absPath)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	// Check for duplicate by file hash (same content, different path)
	if existing == nil {
		dupByHash, err := database.GetBookByHash(hash)
		if err == nil && dupByHash != nil && !ingestForce {
			log.Info("skipped (same file already ingested at different path)",
				"existing_path", dupByHash.SourcePath, "book_id", dupByHash.BookID)
			return nil
		}
	}

	format := detectFormat(absPath)
	if format == "" {
		return fmt.Errorf("unsupported file format")
	}

	log.Info("extracting", "format", format)
	start := time.Now()
	extractCtx, extractCancel := context.WithTimeout(ctx, cfg.Timeouts.WorkerPerFile)
	defer extractCancel()
	resp, err := worker.Extract(extractCtx, absPath, format, cfg.ChunkSize, cfg.ChunkOverlap)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	quality := resp.Quality
	if quality == "" {
		quality = "ok" // backward compat with workers that don't emit quality
	}
	log.Info("extracted", "chapters", len(resp.Chapters), "chunks", len(resp.Chunks), "quality", quality, "duration", time.Since(start).Round(time.Millisecond))

	switch quality {
	case "ocr_required":
		fmt.Printf("  quality: %s — skipping (no extractable text)\n", quality)
		database.LogIngest(0, "failed", fmt.Sprintf("quality=%s: %s", quality, absPath))
		return nil
	case "extract_failed":
		fmt.Printf("  quality: %s — skipping\n", quality)
		database.LogIngest(0, "failed", fmt.Sprintf("quality=%s: %s", quality, absPath))
		return nil
	case "text_corrupt":
		fmt.Printf("  quality: %s — proceeding with warning (text may contain mojibake)\n", quality)
	}

	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	tagsJSON := "[]"
	if len(ingestTags) > 0 {
		tagsJSON = `["` + strings.Join(ingestTags, `","`) + `"]`
	}
	bookRes, err := tx.Exec(
		`INSERT INTO book (title, author, format, source_path, file_hash, tags) VALUES (?, ?, ?, ?, ?, ?)`,
		resp.Book.Title, resp.Book.Author, format, absPath, hash, tagsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert book: %w", err)
	}
	bookID, _ := bookRes.LastInsertId()

	chapterIDMap := make(map[int]int64)
	for _, ch := range resp.Chapters {
		var pageStart, pageEnd sql.NullInt64
		if ch.PageStart != nil {
			pageStart = sql.NullInt64{Int64: int64(*ch.PageStart), Valid: true}
		}
		if ch.PageEnd != nil {
			pageEnd = sql.NullInt64{Int64: int64(*ch.PageEnd), Valid: true}
		}
		res, err := tx.Exec(
			`INSERT INTO chapter (book_id, title, chapter_order, page_start, page_end) VALUES (?, ?, ?, ?, ?)`,
			bookID, ch.Title, ch.Order, pageStart, pageEnd,
		)
		if err != nil {
			return fmt.Errorf("insert chapter %d: %w", ch.Order, err)
		}
		chID, _ := res.LastInsertId()
		chapterIDMap[ch.Order] = chID
	}

	chunkIDs := make([]int64, 0, len(resp.Chunks))
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
		res, err := tx.Exec(
			`INSERT INTO chunk (book_id, chapter_id, heading, body, char_count, page_start, page_end, chunk_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			bookID, chapterID, ck.Heading, ck.Body, ck.CharCount, pageStart, pageEnd, ck.ChunkOrder,
		)
		if err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
		id, _ := res.LastInsertId()
		chunkIDs = append(chunkIDs, id)
	}

	// Link prev/next within each chapter
	prevByChapter := make(map[int64]int64) // chapterID -> last chunk ID
	for i, ck := range resp.Chunks {
		chapterID := chapterIDMap[ck.ChapterOrder]
		chunkID := chunkIDs[i]
		if prevID, exists := prevByChapter[chapterID]; exists {
			if _, err := tx.Exec(`UPDATE chunk SET next_chunk_id = ? WHERE chunk_id = ?`, chunkID, prevID); err != nil {
				return fmt.Errorf("link next: %w", err)
			}
			if _, err := tx.Exec(`UPDATE chunk SET prev_chunk_id = ? WHERE chunk_id = ?`, prevID, chunkID); err != nil {
				return fmt.Errorf("link prev: %w", err)
			}
		}
		prevByChapter[chapterID] = chunkID
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	database.LogIngest(bookID, "chunked", fmt.Sprintf("%d chapters, %d chunks", len(resp.Chapters), len(chunkIDs)))

	log.Info("generating embeddings", "chunks", len(chunkIDs))
	embedStart := time.Now()
	embedFails := 0
	for i, chunkID := range chunkIDs {
		if i > 0 && i%100 == 0 {
			log.Info("embedding progress", "done", i, "total", len(chunkIDs))
		}
		chunkBody := resp.Chunks[i].Body
		emb, err := embedClient.Embed(ctx, chunkBody)
		if err != nil {
			slog.Warn("embedding failed", "chunk_id", chunkID, "error", err)
			embedFails++
			continue
		}
		if err := database.InsertEmbedding(chunkID, emb); err != nil {
			slog.Warn("save embedding failed", "chunk_id", chunkID, "error", err)
			embedFails++
		}
	}

	if embedFails > 0 {
		database.LogIngest(bookID, "embedded", fmt.Sprintf("%d/%d succeeded", len(chunkIDs)-embedFails, len(chunkIDs)))
	}
	database.LogIngest(bookID, "completed", fmt.Sprintf("%d chunks, %d embed failures", len(chunkIDs), embedFails))

	log.Info("done",
		"title", resp.Book.Title,
		"chapters", len(resp.Chapters),
		"chunks", len(chunkIDs),
		"embed_fails", embedFails,
		"embed_duration", time.Since(embedStart).Round(time.Millisecond),
	)
	return nil
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

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func findWorkerPaths() (workerDir, pythonPath string) {
	if cfg.PythonWorkerDir != "" {
		venvPython := filepath.Join(cfg.PythonWorkerDir, "refloom_worker", ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil {
			return cfg.PythonWorkerDir, venvPython
		}
		slog.Warn("configured worker dir has no venv", "dir", cfg.PythonWorkerDir)
		return cfg.PythonWorkerDir, "python3"
	}

	if envDir := os.Getenv("REFLOOM_WORKER_DIR"); envDir != "" {
		venvPython := filepath.Join(envDir, "refloom_worker", ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil {
			return envDir, venvPython
		}
		return envDir, "python3"
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
		if _, err := os.Stat(venvPython); err == nil {
			slog.Debug("found python worker", "dir", absDir)
			return absDir, venvPython
		}
	}

	slog.Warn("python worker venv not found, falling back to system python")
	return "python", "python3"
}
