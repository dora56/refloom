package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/dora56/refloom/internal/db"
	"github.com/spf13/cobra"
)

var (
	inspectChapters bool
	inspectChunkID  int64
	inspectStats    bool
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [book-id]",
	Short: "Inspect book metadata, chapters, and chunks",
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().BoolVar(&inspectChapters, "chapters", false, "Show chapter list")
	inspectCmd.Flags().Int64Var(&inspectChunkID, "chunk", 0, "Show specific chunk details")
	inspectCmd.Flags().BoolVar(&inspectStats, "stats", false, "Show database statistics")
}

func runInspect(cmd *cobra.Command, args []string) error {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	if inspectStats {
		return showStats(database)
	}

	if inspectChunkID > 0 {
		return showChunk(database, inspectChunkID)
	}

	if len(args) == 0 {
		return listBooks(database)
	}

	bookID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid book-id: %s", args[0])
	}

	if inspectChapters {
		return showChapters(database, bookID)
	}
	return showBook(database, bookID)
}

func listBooks(database *db.DB) error {
	books, err := database.ListBooks()
	if err != nil {
		return err
	}
	if len(books) == 0 {
		fmt.Println("No books ingested yet.")
		return nil
	}

	fmt.Printf("%-4s %-6s %-50s %s\n", "ID", "Format", "Title", "Ingested")
	fmt.Println("---- ------ -------------------------------------------------- -------------------")
	for _, b := range books {
		title := b.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Printf("%-4d %-6s %-50s %s\n", b.BookID, b.Format, title, b.IngestedAt)
	}
	return nil
}

func showBook(database *db.DB, bookID int64) error {
	book, err := database.GetBook(bookID)
	if err != nil {
		return err
	}
	if book == nil {
		return fmt.Errorf("book %d not found", bookID)
	}

	chunkCount, _ := database.CountChunksByBook(bookID)
	chapters, _ := database.GetChaptersByBook(bookID)

	fmt.Printf("Book ID:    %d\n", book.BookID)
	fmt.Printf("Title:      %s\n", book.Title)
	fmt.Printf("Author:     %s\n", book.Author)
	fmt.Printf("Format:     %s\n", book.Format)
	fmt.Printf("Source:     %s\n", book.SourcePath)
	fmt.Printf("Tags:       %s\n", book.Tags)
	fmt.Printf("Ingested:   %s\n", book.IngestedAt)
	fmt.Printf("Chapters:   %d\n", len(chapters))
	fmt.Printf("Chunks:     %d\n", chunkCount)
	return nil
}

func showChapters(database *db.DB, bookID int64) error {
	book, err := database.GetBook(bookID)
	if err != nil || book == nil {
		return fmt.Errorf("book %d not found", bookID)
	}

	chapters, err := database.GetChaptersByBook(bookID)
	if err != nil {
		return err
	}

	fmt.Printf("Chapters for: %s\n\n", book.Title)
	for _, ch := range chapters {
		pages := ""
		if ch.PageStart.Valid && ch.PageEnd.Valid {
			pages = fmt.Sprintf("pp.%d-%d", ch.PageStart.Int64, ch.PageEnd.Int64)
		}
		fmt.Printf("  [%d] %s %s\n", ch.ChapterOrder, ch.Title, pages)
	}
	return nil
}

func showChunk(database *db.DB, chunkID int64) error {
	chunk, err := database.GetChunkByID(chunkID)
	if err != nil || chunk == nil {
		return fmt.Errorf("chunk %d not found", chunkID)
	}

	book, _ := database.GetBook(chunk.BookID)
	bookTitle := ""
	if book != nil {
		bookTitle = book.Title
	}

	fmt.Printf("Chunk ID:   %d\n", chunk.ChunkID)
	fmt.Printf("Book:       %s (ID: %d)\n", bookTitle, chunk.BookID)
	fmt.Printf("Chapter ID: %d\n", chunk.ChapterID)
	fmt.Printf("Heading:    %s\n", chunk.Heading)
	if chunk.PageStart.Valid && chunk.PageEnd.Valid {
		fmt.Printf("Pages:      %d-%d\n", chunk.PageStart.Int64, chunk.PageEnd.Int64)
	}
	fmt.Printf("Chars:      %d\n", chunk.CharCount)
	fmt.Printf("Order:      %d\n", chunk.ChunkOrder)

	body := chunk.Body
	if len(body) > 500 {
		body = body[:500] + "..."
	}
	fmt.Printf("\nText:\n%s\n", body)
	return nil
}

func showStats(database *db.DB) error {
	books, _ := database.ListBooks()

	var totalChunks int
	for _, b := range books {
		c, _ := database.CountChunksByBook(b.BookID)
		totalChunks += c
	}

	// Get DB file size
	home, _ := os.UserHomeDir()
	dbPath := home + "/.refloom/refloom.db"
	info, _ := os.Stat(dbPath)
	dbSize := int64(0)
	if info != nil {
		dbSize = info.Size()
	}

	noEmbed, _ := database.CountChunksWithoutEmbedding()

	fmt.Printf("Books:      %d\n", len(books))
	fmt.Printf("Chunks:     %d\n", totalChunks)
	if noEmbed > 0 {
		fmt.Printf("No embed:   %d (run 'refloom reindex --embedding' to fix)\n", noEmbed)
	}
	fmt.Printf("DB Size:    %.1f MB\n", float64(dbSize)/1024/1024)
	return nil
}
