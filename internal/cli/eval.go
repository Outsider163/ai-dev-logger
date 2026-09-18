package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
	"github.com/spf13/cobra"
)

type retrievalCase struct {
	Question        string  `json:"question"`
	ExpectedNoteIDs []int64 `json:"expected_note_ids"`
}

type retrievalCaseResult struct {
	Question         string  `json:"question"`
	Expected         []int64 `json:"expected_note_ids"`
	Retrieved        []int64 `json:"retrieved_note_ids"`
	Hit              bool    `json:"hit"`
	Recall           float64 `json:"recall"`
	ReciprocalRank   float64 `json:"reciprocal_rank"`
	NoAnswerExpected bool    `json:"no_answer_expected"`
	Abstained        bool    `json:"abstained"`
}

func newEvalCommand() *cobra.Command {
	var input string
	options := askOptions{}
	reportOptions := evalReportOptions{}
	command := &cobra.Command{
		Use: "eval --input <cases.json>", Short: "评估已知问题的笔记检索质量",
		Long:    "读取问题和预期笔记编号，按 ask 的多片段选择规则评估检索。\n每题调用一次向量服务（临时失败可能重试），不调用聊天服务。\n输出命中率、平均召回率和 MRR；空预期数组表示无答案题，单独统计正确拒绝率。\n仅评估检索，不判断生成答案是否正确。\n--format json 输出结构化报告；低于任一质量门槛时，输出完整报告后返回失败。",
		Example: "  adl eval --input cases.json --limit 3\n  adl eval --input cases.json --limit 1 --min-score 0.4\n  adl eval --input cases.json --format json --min-hit-rate 0.8 --min-mrr 0.7",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.ConfigPath, options.DBPath = configPath, dbPath
			options.Output, options.Error = cmd.OutOrStdout(), cmd.ErrOrStderr()
			return runRetrievalEval(cmd.Context(), input, options, reportOptions)
		},
	}
	command.Flags().StringVarP(&input, "input", "i", "", "评估 JSON，包含 question 和 expected_note_ids")
	command.Flags().IntVar(&options.Limit, "limit", 5, "最多选择的笔记数，1 到 20")
	command.Flags().IntVar(&options.ChunksPerNote, "chunks-per-note", 2, "每篇最多片段数，1 到 5")
	command.Flags().IntVar(&options.ContextChars, "context-chars", 12000, "资料字符预算，2400 到 48000")
	command.Flags().Float64Var(&options.MinScore, "min-score", .2, "最低余弦相似度，-1 到 1")
	command.Flags().StringVar(&reportOptions.Format, "format", "text", "报告格式：text 或 json")
	command.Flags().Float64Var(&reportOptions.MinHitRate, "min-hit-rate", 0, "最低命中率，0 到 1；低于门槛时返回失败")
	command.Flags().Float64Var(&reportOptions.MinRecall, "min-recall", 0, "最低平均召回率，0 到 1")
	command.Flags().Float64Var(&reportOptions.MinMRR, "min-mrr", 0, "最低 MRR，0 到 1")
	command.Flags().Float64Var(&reportOptions.MinAbstentionRate, "min-abstention-rate", 0, "无答案题最低正确拒绝率，0 到 1")
	_ = command.MarkFlagRequired("input")
	command.AddCommand(newEvalCompareCommand())
	return command
}

func readRetrievalCases(path string) ([]retrievalCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("evaluation file exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	decoder.DisallowUnknownFields()
	var cases []retrievalCase
	if err := decoder.Decode(&cases); err != nil {
		return nil, fmt.Errorf("decode evaluation: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("evaluation file must contain one JSON array")
	}
	if len(cases) == 0 || len(cases) > 50 {
		return nil, fmt.Errorf("evaluation requires 1 to 50 cases")
	}
	for i, item := range cases {
		if strings.TrimSpace(item.Question) == "" || len([]rune(item.Question)) > 4096 {
			return nil, fmt.Errorf("case %d question must contain 1 to 4096 characters", i+1)
		}
		if item.ExpectedNoteIDs == nil || len(item.ExpectedNoteIDs) > 20 {
			return nil, fmt.Errorf("case %d requires an expected_note_ids array with at most 20 IDs; use [] for no-answer cases", i+1)
		}
		seen := make(map[int64]bool)
		for _, id := range item.ExpectedNoteIDs {
			if id <= 0 || seen[id] {
				return nil, fmt.Errorf("case %d has invalid or duplicate note ID %d", i+1, id)
			}
			seen[id] = true
		}
	}
	return cases, nil
}

func runRetrievalEval(ctx context.Context, input string, options askOptions, reportOptions evalReportOptions) error {
	if err := validateEvalReportOptions(reportOptions); err != nil {
		return err
	}
	options.Question = "evaluation"
	if err := validateAskOptions(options); err != nil {
		return err
	}
	cases, err := readRetrievalCases(input)
	if err != nil {
		return err
	}
	positive, negative := 0, 0
	for _, item := range cases {
		if len(item.ExpectedNoteIDs) == 0 {
			negative++
		} else {
			positive++
		}
	}
	if err := validateEvalCoverage(positive, negative, reportOptions); err != nil {
		return err
	}
	cfg, err := appconfig.Load(options.ConfigPath)
	if err != nil {
		return err
	}
	provider := cfg.EmbeddingRuntimeProvider()
	client := llm.NewEmbeddingClient(provider)
	if err := client.ValidateEmbeddingConfig(); err != nil {
		return err
	}
	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	for i, item := range cases {
		for _, id := range item.ExpectedNoteIDs {
			if _, err := db.GetNote(ctx, id); err != nil {
				return fmt.Errorf("case %d expected note #%d: %w", i+1, id, err)
			}
		}
	}
	candidates, err := db.ListEmbeddedNotes(ctx, provider.Model)
	if err != nil {
		return err
	}
	current, stale := selectCurrentSemanticCandidates(candidates)
	if stale > 0 {
		fmt.Fprintf(options.Error, "warning: skipped %d stale embeddings; run adl embed --all\n", stale)
	}
	if len(current) == 0 {
		return fmt.Errorf("no current embeddings for model %q; run adl embed --all", provider.Model)
	}
	results := make([]retrievalCaseResult, 0, len(cases))
	for i, item := range cases {
		vector, err := client.CreateEmbedding(ctx, item.Question)
		if err != nil {
			return fmt.Errorf("evaluate case %d: %w", i+1, err)
		}
		matches, invalid := rankSemanticChunks(vector, current, options.MinScore)
		if len(invalid) == len(current) {
			return fmt.Errorf("case %d: all embeddings incompatible; rebuild with embed --all --force", i+1)
		}
		if len(invalid) > 0 {
			fmt.Fprintf(options.Error, "warning: case %d skipped %d incompatible embeddings\n", i+1, len(invalid))
		}
		sources, _ := selectAskSources(matches, options.Limit, options.ChunksPerNote, options.ContextChars)
		results = append(results, scoreRetrieval(item, sources))
	}
	return writeEvalReport(options.Output, provider.Model, options, reportOptions, results)
}

func scoreRetrieval(item retrievalCase, sources []llm.SearchNote) retrievalCaseResult {
	result := retrievalCaseResult{Question: item.Question, Expected: item.ExpectedNoteIDs, Retrieved: []int64{}, NoAnswerExpected: len(item.ExpectedNoteIDs) == 0}
	expected, seen := make(map[int64]bool), make(map[int64]bool)
	for _, id := range item.ExpectedNoteIDs {
		expected[id] = true
	}
	hits := 0
	for _, source := range sources {
		if seen[source.ID] {
			continue
		}
		seen[source.ID] = true
		result.Retrieved = append(result.Retrieved, source.ID)
		if expected[source.ID] {
			hits++
			if !result.Hit {
				result.ReciprocalRank = 1 / float64(len(result.Retrieved))
				result.Hit = true
			}
		}
	}
	if len(expected) > 0 {
		result.Recall = float64(hits) / float64(len(expected))
	}
	result.Abstained = len(result.Retrieved) == 0
	return result
}
