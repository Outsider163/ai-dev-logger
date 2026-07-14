package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var updateTitle string
var updateBody string
var updateTags []string

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update an existing note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("id must be a positive number")
		}

		titleChanged := cmd.Flags().Changed("title")
		bodyChanged := cmd.Flags().Changed("body")
		tagsChanged := cmd.Flags().Changed("tag")

		var title *string
		if titleChanged {
			cleanTitle := strings.TrimSpace(updateTitle)
			if cleanTitle == "" {
				return fmt.Errorf("title cannot be empty")
			}
			title = &cleanTitle
		}

		bodyValue := updateBody
		if !bodyChanged {
			stdinBody, hasStdin, err := readStdinIfAvailable()
			if err != nil {
				return err
			}
			if hasStdin {
				bodyValue = stdinBody
				bodyChanged = true
			}
		}

		var body *string
		if bodyChanged {
			if strings.TrimSpace(bodyValue) == "" {
				return fmt.Errorf("body cannot be empty")
			}
			body = &bodyValue
		}

		if !titleChanged && !bodyChanged && !tagsChanged {
			return fmt.Errorf("nothing to update, pass --title, --body, --tag, or pipe body from stdin")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		note, err := db.UpdateNote(cmd.Context(), store.UpdateNoteInput{
			ID:          id,
			Title:       title,
			Body:        body,
			Tags:        cleanTags(updateTags),
			ReplaceTags: tagsChanged,
		})
		if errors.Is(err, store.ErrNoteNotFound) {
			return fmt.Errorf("note #%d not found", id)
		}
		if err != nil {
			return err
		}

		fmt.Printf("updated note #%d\n", note.ID)
		return nil
	},
}

func init() {
	updateCmd.Flags().StringVarP(&updateTitle, "title", "t", "", "New note title")
	updateCmd.Flags().StringVarP(&updateBody, "body", "b", "", "New note body, Markdown is supported")
	updateCmd.Flags().StringArrayVar(&updateTags, "tag", nil, "Replacement tag, can be used multiple times")
}

func readStdinIfAvailable() (string, bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", false, err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", false, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false, err
	}
	if len(data) == 0 {
		return "", false, nil
	}
	return string(data), true, nil
}
