package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a full note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("id must be a positive number")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		note, err := db.GetNote(cmd.Context(), id)
		if errors.Is(err, store.ErrNoteNotFound) {
			return fmt.Errorf("note #%d not found", id)
		}
		if err != nil {
			return err
		}

		fmt.Printf("#%d  %s\n", note.ID, note.Title)
		fmt.Printf("created: %s\n", note.CreatedAt.Format("2006-01-02 15:04"))
		if len(note.Tags) > 0 {
			fmt.Printf("tags: %s\n", strings.Join(note.Tags, ", "))
		}
		if strings.TrimSpace(note.Summary) != "" {
			fmt.Printf("summary: %s\n", note.Summary)
		}
		fmt.Println()
		fmt.Println(note.Body)

		return nil
	},
}
