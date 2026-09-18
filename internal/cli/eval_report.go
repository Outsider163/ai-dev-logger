package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

type evalReportOptions struct {
	Format            string
	MinHitRate        float64
	MinRecall         float64
	MinMRR            float64
	MinAbstentionRate float64
}

type evalMetrics struct {
	HitRate        *float64 `json:"hit_rate"`
	MeanRecall     *float64 `json:"mean_recall"`
	MRR            *float64 `json:"mrr"`
	AbstentionRate *float64 `json:"abstention_rate"`
}

type evalThresholds struct {
	HitRate        float64 `json:"hit_rate"`
	MeanRecall     float64 `json:"mean_recall"`
	MRR            float64 `json:"mrr"`
	AbstentionRate float64 `json:"abstention_rate"`
}

type evalSettings struct {
	Limit         int     `json:"limit"`
	ChunksPerNote int     `json:"chunks_per_note"`
	ContextChars  int     `json:"context_chars"`
	MinScore      float64 `json:"min_score"`
}

type evalReport struct {
	SchemaVersion     int                   `json:"schema_version"`
	EmbeddingModel    string                `json:"embedding_model"`
	Settings          evalSettings          `json:"settings"`
	CaseCount         int                   `json:"case_count"`
	AnswerableCount   int                   `json:"answerable_case_count"`
	UnanswerableCount int                   `json:"unanswerable_case_count"`
	Cases             []retrievalCaseResult `json:"cases"`
	Metrics           evalMetrics           `json:"metrics"`
	Thresholds        evalThresholds        `json:"thresholds"`
	Passed            bool                  `json:"passed"`
	Failures          []string              `json:"failures"`
}

func validateEvalReportOptions(options evalReportOptions) error {
	if options.Format != "" && options.Format != "text" && options.Format != "json" {
		return fmt.Errorf("format must be text or json")
	}
	for _, threshold := range []struct {
		name  string
		value float64
	}{
		{"min-hit-rate", options.MinHitRate}, {"min-recall", options.MinRecall}, {"min-mrr", options.MinMRR},
		{"min-abstention-rate", options.MinAbstentionRate},
	} {
		if math.IsNaN(threshold.value) || threshold.value < 0 || threshold.value > 1 {
			return fmt.Errorf("%s must be between 0 and 1", threshold.name)
		}
	}
	return nil
}

func writeEvalReport(output io.Writer, model string, options askOptions, reportOptions evalReportOptions, results []retrievalCaseResult) error {
	if err := validateEvalReportOptions(reportOptions); err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("cannot report an empty evaluation")
	}
	report := evalReport{
		SchemaVersion: 2, EmbeddingModel: model,
		Settings:  evalSettings{Limit: options.Limit, ChunksPerNote: options.ChunksPerNote, ContextChars: options.ContextChars, MinScore: options.MinScore},
		CaseCount: len(results), Cases: results,
		Thresholds: evalThresholds{HitRate: reportOptions.MinHitRate, MeanRecall: reportOptions.MinRecall, MRR: reportOptions.MinMRR, AbstentionRate: reportOptions.MinAbstentionRate},
		Passed:     true, Failures: []string{},
	}
	var hit, recall, mrr, abstentions float64
	for _, result := range results {
		if len(result.Expected) == 0 {
			report.UnanswerableCount++
			if len(result.Retrieved) == 0 {
				abstentions++
			}
			continue
		}
		report.AnswerableCount++
		if result.Hit {
			hit++
		}
		recall += result.Recall
		mrr += result.ReciprocalRank
	}
	if err := validateEvalCoverage(report.AnswerableCount, report.UnanswerableCount, reportOptions); err != nil {
		return err
	}
	report.Metrics = evalMetrics{
		HitRate: evalRatio(hit, report.AnswerableCount), MeanRecall: evalRatio(recall, report.AnswerableCount),
		MRR: evalRatio(mrr, report.AnswerableCount), AbstentionRate: evalRatio(abstentions, report.UnanswerableCount),
	}
	for _, check := range []struct {
		name    string
		actual  *float64
		minimum float64
	}{
		{"hit_rate", report.Metrics.HitRate, reportOptions.MinHitRate},
		{"mean_recall", report.Metrics.MeanRecall, reportOptions.MinRecall},
		{"mrr", report.Metrics.MRR, reportOptions.MinMRR},
		{"abstention_rate", report.Metrics.AbstentionRate, reportOptions.MinAbstentionRate},
	} {
		if check.actual != nil && *check.actual < check.minimum {
			report.Failures = append(report.Failures, fmt.Sprintf("%s %g < %g", check.name, *check.actual, check.minimum))
		}
	}
	report.Passed = len(report.Failures) == 0
	if reportOptions.Format == "json" {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		var text strings.Builder
		fmt.Fprintf(&text, "Retrieval evaluation: %d cases\nembedding model: %s\nlimit: %d; chunks-per-note: %d; min-score: %.4f; context-chars: %d\n", len(results), model, options.Limit, options.ChunksPerNote, options.MinScore, options.ContextChars)
		for i, result := range results {
			if len(result.Expected) == 0 {
				fmt.Fprintf(&text, "\n%d. %s\nexpected: no answer; retrieved: %v\ncorrect abstention: %t\n", i+1, result.Question, result.Retrieved, len(result.Retrieved) == 0)
				continue
			}
			fmt.Fprintf(&text, "\n%d. %s\nexpected: %v; retrieved: %v\nhit: %t; recall: %.4f; reciprocal rank: %.4f\n", i+1, result.Question, result.Expected, result.Retrieved, result.Hit, result.Recall, result.ReciprocalRank)
		}
		fmt.Fprintf(&text, "\nAnswerable cases: %d; no-answer cases: %d\nHit rate: %s\nMean recall: %s\nMRR: %s\nAbstention rate: %s\nQuality gate passed: %t\n", report.AnswerableCount, report.UnanswerableCount, formatEvalMetric(report.Metrics.HitRate), formatEvalMetric(report.Metrics.MeanRecall), formatEvalMetric(report.Metrics.MRR), formatEvalMetric(report.Metrics.AbstentionRate), report.Passed)
		for _, failure := range report.Failures {
			fmt.Fprintf(&text, "FAIL: %s\n", failure)
		}
		if _, err := io.WriteString(output, text.String()); err != nil {
			return err
		}
	}
	if !report.Passed {
		return fmt.Errorf("retrieval quality gate failed: %s", strings.Join(report.Failures, "; "))
	}
	return nil
}

func evalRatio(sum float64, count int) *float64 {
	if count == 0 {
		return nil
	}
	value := sum / float64(count)
	return &value
}

func formatEvalMetric(value *float64) string {
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.4f", *value)
}

func validateEvalCoverage(positive, negative int, options evalReportOptions) error {
	if positive == 0 && (options.MinHitRate > 0 || options.MinRecall > 0 || options.MinMRR > 0) {
		return fmt.Errorf("answerable cases are required for positive hit-rate, recall or MRR thresholds")
	}
	if negative == 0 && options.MinAbstentionRate > 0 {
		return fmt.Errorf("no-answer cases are required for a positive min-abstention-rate")
	}
	return nil
}
