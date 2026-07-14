package cli

import (
	"fmt"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var listLimit int

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "列出最近的开发笔记",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		notes, err := db.ListNotes(cmd.Context(), listLimit)
		if err != nil {
			return err
		}
		if len(notes) == 0 {
			fmt.Println("no notes yet")
			return nil
		}

		for _, note := range notes {
			fmt.Printf("#%d  %s  %s\n", note.ID, note.Title, note.CreatedAt.Format("2006-01-02 15:04"))
			if len(note.Tags) > 0 {
				fmt.Printf("    tags: %s\n", strings.Join(note.Tags, ", "))
			}
			fmt.Printf("    %s\n\n", firstLine(note.Body))
		}

		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Maximum number of notes to show")
}

func firstLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "(empty)"
	}

	line := strings.TrimSpace(lines[0])
	const maxLen = 90
	if len([]rune(line)) <= maxLen {
		return line
	}

	runes := []rune(line)
	return string(runes[:maxLen]) + "..."
}
