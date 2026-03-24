package cli

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/spf13/cobra"
)

var (
	reindexBookID    int64
	reindexEmbedding bool
	reindexFTS       bool
	reindexLinks     bool
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild search indexes (FTS and/or embeddings)",
	RunE:  runReindex,
}

func init() {
	reindexCmd.Flags().Int64Var(&reindexBookID, "book", 0, "Reindex specific book only")
	reindexCmd.Flags().BoolVar(&reindexEmbedding, "embedding", false, "Regenerate embeddings only")
	reindexCmd.Flags().BoolVar(&reindexFTS, "fts", false, "Rebuild FTS index only")
	reindexCmd.Flags().BoolVar(&reindexLinks, "links", false, "Rebuild prev/next chunk links only")
}

func runReindex(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Reindex)
	defer cancel()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	// If --links is set, only rebuild links
	if reindexLinks {
		if err := rebuildChunkLinks(database); err != nil {
			return fmt.Errorf("rebuild links: %w", err)
		}
		slog.Info("reindex complete (links)")
		return nil
	}

	doFTS := !reindexEmbedding || reindexFTS
	doEmb := !reindexFTS || reindexEmbedding

	if doFTS {
		if err := rebuildFTS(database); err != nil {
			return fmt.Errorf("rebuild FTS: %w", err)
		}
	}

	if doEmb {
		if err := rebuildEmbeddings(ctx, database); err != nil {
			return fmt.Errorf("rebuild embeddings: %w", err)
		}
	}

	slog.Info("reindex complete")
	return nil
}

func rebuildFTS(database *db.DB) error {
	slog.Info("rebuilding FTS indexes")
	start := time.Now()

	// Rebuild trigram FTS
	_, err := database.Exec("INSERT INTO chunk_fts(chunk_fts) VALUES('rebuild')")
	if err != nil {
		return fmt.Errorf("rebuild fts trigram: %w", err)
	}

	// Rebuild segmented FTS
	slog.Info("rebuilding segmented FTS index")
	if _, err := database.Exec("DELETE FROM chunk_fts_seg"); err != nil {
		return fmt.Errorf("clear fts_seg: %w", err)
	}

	rows, err := database.Query("SELECT chunk_id, heading, body FROM chunk ORDER BY chunk_id")
	if err != nil {
		return fmt.Errorf("query chunks for fts_seg: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var chunkID int64
		var heading, body string
		if err := rows.Scan(&chunkID, &heading, &body); err != nil {
			return fmt.Errorf("scan chunk: %w", err)
		}
		if err := database.InsertSegmentedFTS(chunkID, heading, body); err != nil {
			slog.Warn("fts_seg insert failed", "chunk_id", chunkID, "error", err)
			continue
		}
		count++
	}

	slog.Info("FTS indexes rebuilt", "segmented_chunks", count, "duration", time.Since(start).Round(time.Millisecond))
	return nil
}

func rebuildEmbeddings(ctx context.Context, database *db.DB) error {
	embedClient := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)
	if err := embedClient.CheckHealth(ctx); err != nil {
		return err
	}

	var chunks []*db.Chunk
	var err error
	if reindexBookID > 0 {
		chunks, err = database.GetChunksByBook(reindexBookID)
		if err != nil {
			return fmt.Errorf("get chunks for book %d: %w", reindexBookID, err)
		}
	} else {
		books, err := database.ListBooks()
		if err != nil {
			return fmt.Errorf("list books: %w", err)
		}
		for _, b := range books {
			bookChunks, err := database.GetChunksByBook(b.BookID)
			if err != nil {
				slog.Warn("failed to get chunks", "book_id", b.BookID, "error", err)
				continue
			}
			chunks = append(chunks, bookChunks...)
		}
	}

	slog.Info("regenerating embeddings", "chunks", len(chunks))
	start := time.Now()
	fails := 0
	for i, chunk := range chunks {
		if i > 0 && i%100 == 0 {
			slog.Info("embedding progress", "done", i, "total", len(chunks))
		}

		database.DeleteEmbedding(chunk.ChunkID)

		emb, err := embedClient.Embed(ctx, chunk.Body)
		if err != nil {
			slog.Warn("embedding failed", "chunk_id", chunk.ChunkID, "error", err)
			fails++
			continue
		}
		if err := database.InsertEmbedding(chunk.ChunkID, emb); err != nil {
			slog.Warn("save embedding failed", "chunk_id", chunk.ChunkID, "error", err)
			fails++
		}
	}

	slog.Info("embeddings regenerated", "total", len(chunks), "fails", fails, "duration", time.Since(start).Round(time.Millisecond))
	return nil
}

func rebuildChunkLinks(database *db.DB) error {
	slog.Info("rebuilding chunk prev/next links")
	start := time.Now()

	// Clear all existing links
	if _, err := database.Exec(`UPDATE chunk SET prev_chunk_id = NULL, next_chunk_id = NULL`); err != nil {
		return fmt.Errorf("clear links: %w", err)
	}

	// Query all chunks ordered by book, chapter, chunk_order
	rows, err := database.Query(
		`SELECT chunk_id, chapter_id FROM chunk ORDER BY book_id, chapter_id, chunk_order`,
	)
	if err != nil {
		return fmt.Errorf("query chunks: %w", err)
	}
	defer rows.Close()

	prevByChapter := make(map[int64]int64) // chapterID -> last chunkID
	linked := 0
	for rows.Next() {
		var chunkID, chapterID int64
		if err := rows.Scan(&chunkID, &chapterID); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if prevID, exists := prevByChapter[chapterID]; exists {
			if _, err := database.Exec(`UPDATE chunk SET next_chunk_id = ? WHERE chunk_id = ?`, chunkID, prevID); err != nil {
				return fmt.Errorf("link next: %w", err)
			}
			if _, err := database.Exec(`UPDATE chunk SET prev_chunk_id = ? WHERE chunk_id = ?`, prevID, chunkID); err != nil {
				return fmt.Errorf("link prev: %w", err)
			}
			linked++
		}
		prevByChapter[chapterID] = chunkID
	}

	slog.Info("chunk links rebuilt", "linked", linked, "duration", time.Since(start).Round(time.Millisecond))
	return nil
}
