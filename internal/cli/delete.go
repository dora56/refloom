package cli

import (
	"fmt"
	"strconv"

	"github.com/dora56/refloom/internal/db"
	"github.com/spf13/cobra"
)

var deleteForce bool

var deleteCmd = &cobra.Command{
	Use:   "delete <book-id>",
	Short: "Delete a book and all its data from the database",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation")
}

func runDelete(cmd *cobra.Command, args []string) error {
	bookID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid book ID: %w", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close() //nolint:errcheck

	book, err := database.GetBook(bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}
	if book == nil {
		return fmt.Errorf("book %d not found", bookID)
	}

	if !deleteForce {
		fmt.Printf("Delete \"%s\" (ID: %d)? [y/N] ", book.Title, bookID)
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil || (answer != "y" && answer != "Y") {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := database.DeleteBook(bookID); err != nil {
		return fmt.Errorf("delete book: %w", err)
	}

	fmt.Printf("Deleted \"%s\" (ID: %d)\n", book.Title, bookID)
	return nil
}
