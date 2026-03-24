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
}

func runReindex(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Reindex)
	defer cancel()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

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
	slog.Info("rebuilding FTS index")
	start := time.Now()

	_, err := database.Exec("INSERT INTO chunk_fts(chunk_fts) VALUES('rebuild')")
	if err != nil {
		return fmt.Errorf("rebuild fts: %w", err)
	}

	slog.Info("FTS index rebuilt", "duration", time.Since(start).Round(time.Millisecond))
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
