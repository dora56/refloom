package cli

import (
	"context"
	"encoding/json"
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
	askLimit         int
	askBookID        int64
	askJSON          bool
	askExpandContext bool
	askHyDE          bool
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
	askCmd.Flags().BoolVar(&askJSON, "json", false, "Output results as JSON")
	askCmd.Flags().BoolVar(&askExpandContext, "expand-context", false, "Include adjacent chunks for richer context")
	askCmd.Flags().BoolVar(&askHyDE, "hyde", false, "Use HyDE (Hypothetical Document Embeddings) for improved retrieval")
}

func runAsk(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeouts.Ask)
	defer cancel()

	query := args[0]

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	embedClient := embedding.NewClient(cfg.OllamaURL, cfg.OllamaEmbedModel)

	// Search for relevant chunks
	engine := search.NewEngine(database, embedClient)
	var bookIDPtr *int64
	if askBookID > 0 {
		bookIDPtr = &askBookID
	}

	// Set up LLM provider (needed before search for HyDE)
	var provider llm.Provider
	switch cfg.LLMProvider {
	case "claude-cli":
		provider = llm.NewClaudeCLI(cfg.AnthropicModel)
	default:
		provider = llm.NewClaude(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	}

	// Generate hypothesis for HyDE (timed separately from retrieval)
	var hypothesis string
	if askHyDE {
		var err error
		hypothesis, err = generateHypothesis(ctx, provider, query)
		if err != nil {
			return fmt.Errorf("generate hypothesis: %w", err)
		}
	}

	retrievalStart := time.Now()
	var results []search.Result
	if askHyDE {
		var err error
		results, err = engine.SearchHybridWithHyDE(ctx, query, hypothesis, askLimit, bookIDPtr)
		if err != nil {
			return fmt.Errorf("hyde search: %w", err)
		}
	} else {
		var err error
		results, err = engine.Search(ctx, query, askLimit, search.ModeHybrid, bookIDPtr)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}
	}
	if askExpandContext {
		search.EnrichWithAdjacentChunks(database, results)
	}
	retrievalMs := time.Since(retrievalStart).Milliseconds()

	if len(results) == 0 {
		if askJSON {
			return printAskJSON(query, "", results, retrievalMs, 0)
		}
		fmt.Println("No relevant passages found.")
		return nil
	}

	// Build prompt and call LLM
	perChunk := cfg.PromptChunkLimit
	if askExpandContext && perChunk < 1200 {
		perChunk = 1200
	}
	promptOpts := citation.PromptOptions{
		Budget:        cfg.PromptBudget,
		PerChunk:      perChunk,
		ExpandContext: askExpandContext,
	}
	system, user := citation.BuildPromptWithBudget(query, results, promptOpts)

	genStart := time.Now()
	answer, err := provider.Generate(ctx, system, user)
	if err != nil {
		return fmt.Errorf("generate answer: %w", err)
	}
	generationMs := time.Since(genStart).Milliseconds()

	if askJSON {
		return printAskJSON(query, answer, results, retrievalMs, generationMs)
	}

	// Print answer and sources
	fmt.Println("Answer:")
	fmt.Println(answer)
	fmt.Println()
	fmt.Println(citation.FormatSources(results))

	return nil
}

// askSourceJSON is the JSON format for a source in ask output.
type askSourceJSON struct {
	BookTitle string `json:"book_title"`
	Chapter   string `json:"chapter"`
	PageStart *int64 `json:"page_start,omitempty"`
	PageEnd   *int64 `json:"page_end,omitempty"`
}

func printAskJSON(query, answer string, results []search.Result, retrievalMs, generationMs int64) error {
	sources := make([]askSourceJSON, 0, len(results))
	for _, r := range results {
		s := askSourceJSON{}
		if r.Book != nil {
			s.BookTitle = r.Book.Title
		}
		if r.Chapter != nil {
			s.Chapter = r.Chapter.Title
		}
		if r.Chunk != nil {
			if r.Chunk.PageStart.Valid {
				v := r.Chunk.PageStart.Int64
				s.PageStart = &v
			}
			if r.Chunk.PageEnd.Valid {
				v := r.Chunk.PageEnd.Int64
				s.PageEnd = &v
			}
		}
		sources = append(sources, s)
	}

	out := struct {
		Query        string          `json:"query"`
		Answer       string          `json:"answer"`
		Sources      []askSourceJSON `json:"sources"`
		RetrievalMs  int64           `json:"retrieval_ms"`
		GenerationMs int64           `json:"generation_ms"`
		TotalMs      int64           `json:"total_ms"`
	}{
		Query:        query,
		Answer:       answer,
		Sources:      sources,
		RetrievalMs:  retrievalMs,
		GenerationMs: generationMs,
		TotalMs:      retrievalMs + generationMs,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

const hydeSystemPrompt = `あなたは日本語の技術書の専門家です。
ユーザーの質問に対して、技術書に書かれていそうな仮説的な回答を日本語で生成してください。
実際の正確さより、関連する技術用語や概念を豊富に含むことを重視してください。
200〜400文字程度で回答してください。`

func generateHypothesis(ctx context.Context, provider llm.Provider, query string) (string, error) {
	return provider.Generate(ctx, hydeSystemPrompt, query)
}
