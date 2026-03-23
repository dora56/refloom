package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/dora56/refloom/internal/citation"
	"github.com/dora56/refloom/internal/db"
	"github.com/dora56/refloom/internal/embedding"
	"github.com/dora56/refloom/internal/llm"
	"github.com/dora56/refloom/internal/search"
	"github.com/spf13/cobra"
)

var (
	askLimit  int
	askBookID int64
)

var askCmd = &cobra.Command{
	Use:   "ask <query>",
	Short: "Ask a question and get a summary-based answer with citations",
	Args:  cobra.ExactArgs(1),
	RunE:  runAsk,
}

func init() {
	askCmd.Flags().IntVar(&askLimit, "limit", 5, "Number of source chunks to use")
	askCmd.Flags().Int64Var(&askBookID, "book", 0, "Limit to specific book ID")
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Search for relevant chunks
	engine := search.NewEngine(database, embedClient)
	var bookIDPtr *int64
	if askBookID > 0 {
		bookIDPtr = &askBookID
	}

	results, err := engine.Search(ctx, query, askLimit, search.ModeHybrid, bookIDPtr)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No relevant passages found.")
		return nil
	}

	// Build prompt and call LLM
	system, user := citation.BuildPrompt(query, results)

	provider := llm.NewClaude("", "")
	answer, err := provider.Generate(ctx, system, user)
	if err != nil {
		return fmt.Errorf("generate answer: %w", err)
	}

	// Print answer and sources
	fmt.Println("Answer:")
	fmt.Println(answer)
	fmt.Println()
	fmt.Println(citation.FormatSources(results))

	return nil
}
