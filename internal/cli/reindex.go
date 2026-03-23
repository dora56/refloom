package cli

import (
	"context"
	"fmt"
	"os"
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	// Default: both
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

	fmt.Println("Reindex complete.")
	return nil
}

func rebuildFTS(database *db.DB) error {
	fmt.Println("Rebuilding FTS index...")

	// Rebuild content sync
	_, err := database.Exec("INSERT INTO chunk_fts(chunk_fts) VALUES('rebuild')")
	if err != nil {
		return fmt.Errorf("rebuild fts: %w", err)
	}

	fmt.Println("  FTS index rebuilt.")
	return nil
}

func rebuildEmbeddings(ctx context.Context, database *db.DB) error {
	embedClient := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)
	if err := embedClient.CheckHealth(ctx); err != nil {
		return err
	}

	// Get chunks to reindex
	var chunks []*db.Chunk
	if reindexBookID > 0 {
		chunks, _ = database.GetChunksByBook(reindexBookID)
	} else {
		// Get all books, then all chunks
		books, _ := database.ListBooks()
		for _, b := range books {
			bookChunks, _ := database.GetChunksByBook(b.BookID)
			chunks = append(chunks, bookChunks...)
		}
	}

	fmt.Printf("  Regenerating embeddings for %d chunks...\n", len(chunks))
	for i, chunk := range chunks {
		if i > 0 && i%50 == 0 {
			fmt.Printf("    %d/%d\n", i, len(chunks))
		}

		// Delete existing embedding
		database.DeleteEmbedding(chunk.ChunkID)

		// Generate new embedding
		emb, err := embedClient.Embed(ctx, chunk.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: embedding failed for chunk %d: %v\n", chunk.ChunkID, err)
			continue
		}
		if err := database.InsertEmbedding(chunk.ChunkID, emb); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: save embedding failed for chunk %d: %v\n", chunk.ChunkID, err)
		}
	}

	fmt.Printf("  Embeddings regenerated: %d chunks\n", len(chunks))
	return nil
}
