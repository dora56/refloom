package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/dora56/refloom/internal/search"
	"github.com/spf13/cobra"
)

var (
	searchLimit  int
	searchMode   string
	searchBookID int64
	searchJSON   bool
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
	searchCmd.Flags().BoolVar(&searchJSON, "json", false, "Output results as JSON")
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Search)
	defer cancel()

	query := args[0]

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	embedClient := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)

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

	if searchJSON {
		return printSearchJSON(results, query, string(mode))
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

// searchResultJSON is the JSON output format for a single search result.
type searchResultJSON struct {
	ChunkID   int64   `json:"chunk_id"`
	Score     float64 `json:"score"`
	BookTitle string  `json:"book_title"`
	Chapter   string  `json:"chapter"`
	PageStart *int64  `json:"page_start,omitempty"`
	PageEnd   *int64  `json:"page_end,omitempty"`
	Preview   string  `json:"preview"`
}

func printSearchJSON(results []search.Result, query, mode string) error {
	items := make([]searchResultJSON, 0, len(results))
	for _, r := range results {
		item := searchResultJSON{
			ChunkID: r.ChunkID,
			Score:   r.Score,
		}
		if r.Book != nil {
			item.BookTitle = r.Book.Title
		}
		if r.Chapter != nil {
			item.Chapter = r.Chapter.Title
		}
		if r.Chunk != nil {
			if r.Chunk.PageStart.Valid {
				v := r.Chunk.PageStart.Int64
				item.PageStart = &v
			}
			if r.Chunk.PageEnd.Valid {
				v := r.Chunk.PageEnd.Int64
				item.PageEnd = &v
			}
			item.Preview = r.Chunk.Body
			if len(item.Preview) > 300 {
				item.Preview = item.Preview[:300]
			}
		}
		items = append(items, item)
	}

	out := struct {
		Query   string             `json:"query"`
		Mode    string             `json:"mode"`
		Count   int                `json:"count"`
		Results []searchResultJSON `json:"results"`
	}{
		Query:   query,
		Mode:    mode,
		Count:   len(items),
		Results: items,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
