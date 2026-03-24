package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dora56/refloom/internal/db"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportBookID int64
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export chunks and metadata as JSON or CSV",
	RunE:  runExport,
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "Output format: json or csv")
	exportCmd.Flags().Int64Var(&exportBookID, "book", 0, "Export specific book only")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")
}

type exportChunk struct {
	BookID    int64  `json:"book_id"`
	BookTitle string `json:"book_title"`
	ChapterID int64  `json:"chapter_id"`
	ChunkID   int64  `json:"chunk_id"`
	Heading   string `json:"heading"`
	Body      string `json:"body"`
	CharCount int    `json:"char_count"`
	Order     int    `json:"chunk_order"`
}

func runExport(cmd *cobra.Command, args []string) error {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	books, err := database.ListBooks()
	if err != nil {
		return fmt.Errorf("list books: %w", err)
	}

	var rows []exportChunk
	for _, b := range books {
		if exportBookID > 0 && b.BookID != exportBookID {
			continue
		}
		chunks, err := database.GetChunksByBook(b.BookID)
		if err != nil {
			return fmt.Errorf("get chunks for book %d: %w", b.BookID, err)
		}
		for _, c := range chunks {
			rows = append(rows, exportChunk{
				BookID:    b.BookID,
				BookTitle: b.Title,
				ChapterID: c.ChapterID,
				ChunkID:   c.ChunkID,
				Heading:   c.Heading,
				Body:      c.Body,
				CharCount: c.CharCount,
				Order:     c.ChunkOrder,
			})
		}
	}

	w := os.Stdout
	if exportOutput != "" {
		f, err := os.Create(exportOutput) //nolint:gosec
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close() //nolint:errcheck
		w = f
	}

	switch exportFormat {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	case "csv":
		return writeCSV(w, rows)
	default:
		return fmt.Errorf("unsupported format: %s (use json or csv)", exportFormat)
	}
}

func writeCSV(w *os.File, rows []exportChunk) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"book_id", "book_title", "chapter_id", "chunk_id", "heading", "body", "char_count", "chunk_order"}); err != nil {
		return err
	}

	for _, r := range rows {
		if err := writer.Write([]string{
			fmt.Sprintf("%d", r.BookID),
			r.BookTitle,
			fmt.Sprintf("%d", r.ChapterID),
			fmt.Sprintf("%d", r.ChunkID),
			r.Heading,
			r.Body,
			fmt.Sprintf("%d", r.CharCount),
			fmt.Sprintf("%d", r.Order),
		}); err != nil {
			return err
		}
	}
	return nil
}
