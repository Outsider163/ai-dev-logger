package cli

import (
	"errors"
	"fmt"
	"strconv"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var deleteYes bool

var deleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Short:   "永久删除一条笔记",
	Long:    "删除笔记及其向量，操作不可撤销。\n先用 adl show <id> 核对笔记，再传入 --yes 确认。",
	Example: "  adl show 1\n  adl delete 1 --yes",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("id must be a positive number")
		}
		if !deleteYes {
			return fmt.Errorf("delete is permanent, pass --yes to confirm")
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.DeleteNote(cmd.Context(), id); errors.Is(err, store.ErrNoteNotFound) {
			return fmt.Errorf("note #%d not found", id)
		} else if err != nil {
			return err
		}

		fmt.Printf("deleted note #%d\n", id)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteYes, "yes", false, "确认永久删除")
}
