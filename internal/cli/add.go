package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var addTitle string
var addBody string
var addTags []string

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "添加一条开发笔记",
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.TrimSpace(addTitle)
		if title == "" {
			return fmt.Errorf("title is required, example: ai-dev-logger add --title \"Go map 踩坑\"")
		}

		body, err := readBody()
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("body is required, pass --body or pipe content from stdin")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		note, err := db.CreateNote(cmd.Context(), store.CreateNoteInput{
			Title: title,
			Body:  body,
			Tags:  cleanTags(addTags),
		})
		if err != nil {
			return err
		}

		fmt.Printf("saved note #%d\n", note.ID)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addTitle, "title", "t", "", "Note title")
	addCmd.Flags().StringVarP(&addBody, "body", "b", "", "Note body, Markdown is supported")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tag, can be used multiple times")
}

func readBody() (string, error) {
	if strings.TrimSpace(addBody) != "" {
		return addBody, nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cleanTags(tags []string) []string {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, tag)
	}

	return cleaned
}
