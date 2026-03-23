package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Find book files
	files, err := findBookFiles(args[0])
	if err != nil {
		return fmt.Errorf("find files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no PDF/EPUB files found at %s", args[0])
	}

	// Open database
	database, err := db.Open("")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	// Setup Python worker
	workerDir, pythonPath := findWorkerPaths()
	worker := extraction.NewWorker(pythonPath, workerDir)

	// Setup embedding client
	// TODO: make model configurable via config file
	embeddingModel := os.Getenv("REFLOOM_EMBEDDING_MODEL")
	if embeddingModel == "" {
		embeddingModel = "nomic-embed-text"
	}
	embedClient := embedding.NewClient("http://localhost:11434", embeddingModel)
	if err := embedClient.CheckHealth(ctx); err != nil {
		return fmt.Errorf("ollama check: %w", err)
	}

	// Process each file
	for i, file := range files {
		fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(files), filepath.Base(file))
		if err := ingestFile(ctx, database, worker, embedClient, file); err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
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

	// Check if already ingested
	existing, err := database.GetBookByPath(absPath)
	if err != nil {
		return err
	}
	if existing != nil && !ingestForce {
		fmt.Printf("  Skipped (already ingested, use --force to re-ingest)\n")
		return nil
	}
	if existing != nil && ingestForce {
		if err := database.DeleteBook(existing.BookID); err != nil {
			return fmt.Errorf("delete existing: %w", err)
		}
	}

	// Compute file hash
	hash, err := fileHash(absPath)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	// Determine format
	format := detectFormat(absPath)
	if format == "" {
		return fmt.Errorf("unsupported file format")
	}

	// Extract via Python worker
	fmt.Printf("  Extracting...\n")
	resp, err := worker.Extract(ctx, absPath, format, 500, 100)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	fmt.Printf("  Extracted: %d chapters, %d chunks\n", len(resp.Chapters), len(resp.Chunks))

	// Begin transaction
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert book
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

	// Insert chapters and build order->ID map
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

	// Insert chunks
	chunkIDs := make([]int64, 0, len(resp.Chunks))
	for _, ck := range resp.Chunks {
		chapterID, ok := chapterIDMap[ck.ChapterOrder]
		if !ok {
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

	// Commit metadata
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Generate embeddings (outside transaction for progress reporting)
	fmt.Printf("  Generating embeddings for %d chunks...\n", len(chunkIDs))
	for i, chunkID := range chunkIDs {
		if i > 0 && i%50 == 0 {
			fmt.Printf("    %d/%d\n", i, len(chunkIDs))
		}
		chunkBody := resp.Chunks[i].Body
		emb, err := embedClient.Embed(ctx, chunkBody)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: embedding failed for chunk %d: %v\n", chunkID, err)
			continue
		}
		if err := database.InsertEmbedding(chunkID, emb); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: save embedding failed for chunk %d: %v\n", chunkID, err)
		}
	}

	fmt.Printf("  Done: \"%s\" — %d chapters, %d chunks\n", resp.Book.Title, len(resp.Chapters), len(chunkIDs))
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
	// Allow override via env
	if envDir := os.Getenv("REFLOOM_WORKER_DIR"); envDir != "" {
		venvPython := filepath.Join(envDir, "refloom_worker", ".venv", "bin", "python3")
		if _, err := os.Stat(venvPython); err == nil {
			return envDir, venvPython
		}
		return envDir, "python3"
	}

	// Try relative to executable
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
			return absDir, venvPython
		}
	}

	// Fallback: system python
	return "python", "python3"
}
