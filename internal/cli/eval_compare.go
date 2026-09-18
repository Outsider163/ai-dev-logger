package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"

	"ai-dev-logger/internal/llm"
	"github.com/spf13/cobra"
)

func newEvalCompareCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "compare <before.json> <after.json>",
		Short:   "本地对比两份检索评估报告",
		Example: "  adl eval compare before.json after.json",
		Long:    "对比版本 2 的 JSON 评估报告，显示指标变化与退步题目。\n要求问题顺序及预期笔记集合相同；不会调用 API 或打开数据库。\n分数下降仅展示，不返回失败；输入错误返回失败。",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			before, err := readEvalComparisonReport(args[0])
			if err != nil {
				return fmt.Errorf("before report: %w", err)
			}
			after, err := readEvalComparisonReport(args[1])
			if err != nil {
				return fmt.Errorf("after report: %w", err)
			}
			return compareEvalReports(cmd.OutOrStdout(), before, after)
		},
	}
}

func readEvalComparisonReport(path string) (evalReport, error) {
	var report evalReport
	file, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer file.Close()
	const maxSize = 2 << 20
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return report, err
	}
	if len(data) > maxSize {
		return report, fmt.Errorf("report exceeds 2 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return report, fmt.Errorf("report must contain one JSON object")
	}
	if report.SchemaVersion != 2 {
		return report, fmt.Errorf("only report schema_version 2 is supported; regenerate the report")
	}
	if len(report.Cases) < 1 || len(report.Cases) > 50 || report.CaseCount != len(report.Cases) {
		return report, fmt.Errorf("invalid case count")
	}
	if strings.TrimSpace(report.EmbeddingModel) == "" {
		return report, fmt.Errorf("missing embedding model")
	}
	options := askOptions{Question: "comparison", Limit: report.Settings.Limit, ChunksPerNote: report.Settings.ChunksPerNote, ContextChars: report.Settings.ContextChars, MinScore: report.Settings.MinScore}
	if err := validateAskOptions(options); err != nil {
		return report, err
	}
	for i, item := range report.Cases {
		if strings.TrimSpace(item.Question) == "" || len([]rune(item.Question)) > 4096 || item.Expected == nil || item.Retrieved == nil || len(item.Expected) > 20 || len(item.Retrieved) > report.Settings.Limit {
			return report, fmt.Errorf("case %d has invalid question or note arrays", i+1)
		}
		for _, ids := range [][]int64{item.Expected, item.Retrieved} {
			seen := make(map[int64]bool)
			for _, id := range ids {
				if id <= 0 || seen[id] {
					return report, fmt.Errorf("case %d has invalid or duplicate note IDs", i+1)
				}
				seen[id] = true
			}
		}
		sources := make([]llm.SearchNote, len(item.Retrieved))
		for j, id := range item.Retrieved {
			sources[j].ID = id
		}
		computed := scoreRetrieval(retrievalCase{Question: item.Question, ExpectedNoteIDs: item.Expected}, sources)
		if !reflect.DeepEqual(computed, item) {
			return report, fmt.Errorf("case %d scores do not match retrieved IDs", i+1)
		}
	}
	// Recompute aggregates so edited or incomplete reports cannot mislead comparisons.
	var normalized bytes.Buffer
	if err := writeEvalReport(&normalized, report.EmbeddingModel, options, evalReportOptions{Format: "json"}, report.Cases); err != nil {
		return report, err
	}
	var computed evalReport
	if err := json.Unmarshal(normalized.Bytes(), &computed); err != nil {
		return report, err
	}
	if report.AnswerableCount != computed.AnswerableCount || report.UnanswerableCount != computed.UnanswerableCount || !reflect.DeepEqual(report.Metrics, computed.Metrics) {
		return report, fmt.Errorf("aggregate metrics do not match case results")
	}
	return report, nil
}

func compareEvalReports(output io.Writer, before, after evalReport) error {
	if len(before.Cases) != len(after.Cases) {
		return fmt.Errorf("reports must use the same ordered evaluation cases")
	}
	for i, old := range before.Cases {
		current := after.Cases[i]
		oldIDs, newIDs := slices.Clone(old.Expected), slices.Clone(current.Expected)
		slices.Sort(oldIDs)
		slices.Sort(newIDs)
		if old.Question != current.Question || !slices.Equal(oldIDs, newIDs) {
			return fmt.Errorf("case %d differs; reports must use the same questions, order and expected note IDs", i+1)
		}
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Retrieval comparison: %d cases\nModel: %s -> %s\n", len(before.Cases), before.EmbeddingModel, after.EmbeddingModel)
	fmt.Fprintf(&text, "Before: limit=%d chunks-per-note=%d context-chars=%d min-score=%g\n", before.Settings.Limit, before.Settings.ChunksPerNote, before.Settings.ContextChars, before.Settings.MinScore)
	fmt.Fprintf(&text, "After:  limit=%d chunks-per-note=%d context-chars=%d min-score=%g\n", after.Settings.Limit, after.Settings.ChunksPerNote, after.Settings.ContextChars, after.Settings.MinScore)
	for _, metric := range []struct {
		name         string
		old, current *float64
	}{
		{"Hit rate", before.Metrics.HitRate, after.Metrics.HitRate},
		{"Mean recall", before.Metrics.MeanRecall, after.Metrics.MeanRecall},
		{"MRR", before.Metrics.MRR, after.Metrics.MRR},
		{"Abstention rate", before.Metrics.AbstentionRate, after.Metrics.AbstentionRate},
	} {
		if metric.old == nil || metric.current == nil {
			fmt.Fprintf(&text, "%s: N/A\n", metric.name)
			continue
		}
		fmt.Fprintf(&text, "%s: %.4f -> %.4f (delta %+.4f)\n", metric.name, *metric.old, *metric.current, *metric.current-*metric.old)
	}
	regressions := 0
	for i, old := range before.Cases {
		current := after.Cases[i]
		regressed := current.Recall < old.Recall || current.ReciprocalRank < old.ReciprocalRank
		if old.NoAnswerExpected {
			regressed = old.Abstained && !current.Abstained
		}
		if regressed {
			regressions++
		}
		if slices.Equal(old.Retrieved, current.Retrieved) {
			continue
		}
		status := "changed"
		if regressed {
			status = "REGRESSION"
		}
		fmt.Fprintf(&text, "\nCase %d [%s]: %s\nretrieved: %v -> %v\n", i+1, status, old.Question, old.Retrieved, current.Retrieved)
		if !old.NoAnswerExpected {
			fmt.Fprintf(&text, "recall: %.4f -> %.4f; reciprocal rank: %.4f -> %.4f\n", old.Recall, current.Recall, old.ReciprocalRank, current.ReciprocalRank)
		}
	}
	fmt.Fprintf(&text, "\nRegressed cases: %d\nNote: reports do not verify that database contents or provider endpoints were unchanged.\n", regressions)
	_, err := io.WriteString(output, text.String())
	return err
}
