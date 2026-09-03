package cli

import (
	"fmt"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var searchLimit int

var searchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "按关键词搜索本地笔记",
	Long:    "在标题、正文和标签中匹配关键词，不调用 AI。\n包含空格的查询请使用引号；按语义搜索请使用 adl semantic。",
	Example: "  adl search \"SQLite\"\n  adl search \"connection timeout\" --limit 5",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(args[0])
		if query == "" {
			return fmt.Errorf("query is required")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		notes, err := db.SearchNotes(cmd.Context(), query, searchLimit)
		if err != nil {
			return err
		}
		if len(notes) == 0 {
			fmt.Println("no matching notes")
			return nil
		}

		for _, note := range notes {
			fmt.Printf("#%d  %s\n", note.ID, note.Title)
			if len(note.Tags) > 0 {
				fmt.Printf("    tags: %s\n", strings.Join(note.Tags, ", "))
			}
			fmt.Printf("    %s\n\n", firstLine(note.Body))
		}

		return nil
	},
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "最多显示的匹配条数")
}
