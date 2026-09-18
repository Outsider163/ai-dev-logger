package cli

import "github.com/spf13/cobra"

var askLimit int
var askMinScore float64
var askChunksPerNote int
var askContextChars int
var askRetrieveOnly bool

var askCmd = &cobra.Command{
	Use:     "ask <question>",
	Short:   "根据本地笔记回答问题并显示来源",
	Long:    "先用向量服务从本地笔记检索片段，再由聊天服务依据这些片段回答。\n回答必须引用 [Note #编号]；未找到足够相关资料时不会调用聊天服务。\n--retrieve-only 仅展示检索资料，不调用聊天服务；仍会调用向量服务。",
	Example: "  adl ask \"以前如何处理 SQLite 锁冲突？\"\n  adl ask \"Go map 并发读写怎么处理？\" --limit 3 --min-score 0.4\n  adl ask \"问题原因和解决方法\" --retrieve-only --chunks-per-note 3",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAsk(cmd.Context(), askOptions{
			ConfigPath: configPath, DBPath: dbPath, Question: args[0], Limit: askLimit,
			MinScore: askMinScore, Output: cmd.OutOrStdout(), Error: cmd.ErrOrStderr(),
			ChunksPerNote: askChunksPerNote, ContextChars: askContextChars,
			RetrieveOnly: askRetrieveOnly,
		})
	},
}

func init() {
	askCmd.Flags().IntVar(&askLimit, "limit", 5, "最多使用多少条笔记作为来源，范围 1 到 20")
	askCmd.Flags().IntVar(&askChunksPerNote, "chunks-per-note", 2, "每篇笔记最多选取多少个相关片段，范围 1 到 5")
	askCmd.Flags().IntVar(&askContextChars, "context-chars", 12000, "检索资料字符预算，含标题/标签/摘要，范围 2400 到 48000；不是 token 数")
	askCmd.Flags().Float64Var(&askMinScore, "min-score", 0.2, "最低余弦相似度，范围为 -1 到 1")
	askCmd.Flags().BoolVar(&askRetrieveOnly, "retrieve-only", false, "展示检索片段和资料文本，不生成回答；仍调用向量服务")
}
