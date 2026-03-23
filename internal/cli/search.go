package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/dora56/refloom/internal/search"
	"github.com/spf13/cobra"
)

var (
	searchLimit  int
	searchMode   string
	searchBookID int64
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search books by keyword or semantic similarity",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "Number of results")
	searchCmd.Flags().StringVar(&searchMode, "mode", "hybrid", "Search mode: fts, vector, or hybrid")
	searchCmd.Flags().Int64Var(&searchBookID, "book", 0, "Limit to specific book ID")
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := args[0]

	database, err := db.Open("")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	embeddingModel := os.Getenv("REFLOOM_EMBEDDING_MODEL")
	if embeddingModel == "" {
		embeddingModel = "nomic-embed-text"
	}
	embedClient := embedding.NewClient("http://localhost:11434", embeddingModel)

	engine := search.NewEngine(database, embedClient)

	var bookIDPtr *int64
	if searchBookID > 0 {
		bookIDPtr = &searchBookID
	}

	mode := search.Mode(searchMode)
	results, err := engine.Search(ctx, query, searchLimit, mode, bookIDPtr)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for i, r := range results {
		fmt.Printf("[%d] Score: %.4f\n", i+1, r.Score)
		if r.Book != nil {
			fmt.Printf("    Book: %s\n", r.Book.Title)
		}
		if r.Chapter != nil {
			fmt.Printf("    Chapter: %s\n", r.Chapter.Title)
		}
		if r.Chunk != nil {
			pageInfo := ""
			if r.Chunk.PageStart.Valid && r.Chunk.PageEnd.Valid {
				pageInfo = fmt.Sprintf("pp.%d-%d", r.Chunk.PageStart.Int64, r.Chunk.PageEnd.Int64)
			}
			if pageInfo != "" {
				fmt.Printf("    Pages: %s\n", pageInfo)
			}
			preview := r.Chunk.Body
			if len(preview) > 150 {
				preview = preview[:150] + "..."
			}
			fmt.Printf("    Preview: %s\n", preview)
		}
		fmt.Println()
	}

	return nil
}
