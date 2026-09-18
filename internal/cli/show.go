package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:     "show <id>",
	Short:   "查看一条笔记的完整内容",
	Example: "  adl show 1\n  adl show 1 --chunks",
	Args:    cobra.ExactArgs(1),
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
		if showChunks {
			return printNoteChunks(cmd.Context(), cmd.OutOrStdout(), db, id)
		}

		note, err := db.GetNote(cmd.Context(), id)
		if errors.Is(err, store.ErrNoteNotFound) {
			return fmt.Errorf("note #%d not found", id)
		}
		if err != nil {
			return err
		}

		fmt.Printf("#%d  %s\n", note.ID, note.Title)
		fmt.Printf("created: %s\n", note.CreatedAt.Format("2006-01-02 15:04"))
		source, err := db.NoteSource(cmd.Context(), id)
		if err != nil {
			return err
		}
		if source != "" {
			fmt.Printf("source: %s\n", source)
		}
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

var showChunks bool

func init() {
	showCmd.Flags().BoolVar(&showChunks, "chunks", false, "查看自动生成的笔记切片，不调用 AI")
}

func printNoteChunks(ctx context.Context, output io.Writer, db *store.Store, id int64) error {
	chunks, err := db.ListChunks(ctx, id)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "note #%d: %d chunks\n", id, len(chunks))
	for _, chunk := range chunks {
		fmt.Fprintf(output, "\n[Note #%d / Chunk %d] (%d characters)\n%s\n", id, chunk.Index+1, len([]rune(chunk.Content)), chunk.Content)
	}
	return nil
}
