package cli

import "github.com/spf13/cobra"

var askLimit int
var askMinScore float64

var askCmd = &cobra.Command{
	Use:     "ask <question>",
	Short:   "根据本地笔记回答问题并显示来源",
	Long:    "先用向量服务从本地笔记检索片段，再由聊天服务依据这些片段回答。\n回答必须引用 [Note #编号]；未找到足够相关资料时不会调用聊天服务。",
	Example: "  adl ask \"以前如何处理 SQLite 锁冲突？\"\n  adl ask \"Go map 并发读写怎么处理？\" --limit 3 --min-score 0.4",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAsk(cmd.Context(), askOptions{
			ConfigPath: configPath, DBPath: dbPath, Question: args[0], Limit: askLimit,
			MinScore: askMinScore, Output: cmd.OutOrStdout(), Error: cmd.ErrOrStderr(),
		})
	},
}

func init() {
	askCmd.Flags().IntVar(&askLimit, "limit", 5, "最多使用多少条笔记作为来源")
	askCmd.Flags().Float64Var(&askMinScore, "min-score", 0.2, "最低余弦相似度，范围为 -1 到 1")
}
