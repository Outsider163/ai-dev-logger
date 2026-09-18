package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestEvalReportThresholdBoundariesAndPrecision(t *testing.T) {
	results := []retrievalCaseResult{
		{Expected: []int64{1}, Hit: true, Recall: 1, ReciprocalRank: 1},
		{Expected: []int64{1}, Hit: true, Recall: 1, ReciprocalRank: 1},
		{Expected: []int64{1}, Retrieved: []int64{}},
	}
	for _, tc := range []struct {
		name    string
		minimum float64
		passed  bool
	}{
		{"disabled", 0, true}, {"exact", 2.0 / 3, true}, {"rounded", .6667, false}, {"higher", .8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := writeEvalReport(&output, "embed", askOptions{Limit: 3, ChunksPerNote: 2, ContextChars: 12000, MinScore: .2}, evalReportOptions{Format: "json", MinHitRate: tc.minimum, MinRecall: tc.minimum, MinMRR: tc.minimum}, results)
			if (err == nil) != tc.passed {
				t.Fatalf("gate: %v", err)
			}
			var report evalReport
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.SchemaVersion != 2 || report.Passed != tc.passed || report.Settings.Limit != 3 || report.Metrics.MRR == nil || *report.Metrics.MRR != 2.0/3 {
				t.Fatalf("report: %+v", report)
			}
			if !tc.passed && len(report.Failures) != 3 {
				t.Fatalf("missing threshold failures: %v", report.Failures)
			}
			if !strings.Contains(output.String(), `"retrieved_note_ids": []`) {
				t.Fatal("empty retrieval must be an array")
			}
		})
	}
}

func TestEvalReportRejectsInvalidOptionsBeforeReadingInputs(t *testing.T) {
	for _, options := range []evalReportOptions{
		{Format: "xml"}, {MinHitRate: -1}, {MinRecall: 1.1}, {MinMRR: math.NaN()}, {MinHitRate: math.Inf(1)},
		{MinAbstentionRate: -1}, {MinAbstentionRate: math.NaN()},
	} {
		err := runRetrievalEval(context.Background(), "missing-input.json", askOptions{}, options)
		if err == nil || (!strings.Contains(err.Error(), "must be between") && !strings.Contains(err.Error(), "format must")) {
			t.Fatalf("validation: %v", err)
		}
	}
}

type evalFailWriter struct{ err error }

func (w evalFailWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestEvalReportPropagatesWriteFailure(t *testing.T) {
	want := errors.New("output unavailable")
	for _, format := range []string{"text", "json"} {
		err := writeEvalReport(evalFailWriter{want}, "embed", askOptions{}, evalReportOptions{Format: format, MinMRR: 1}, []retrievalCaseResult{{Expected: []int64{1}}})
		if !errors.Is(err, want) {
			t.Fatalf("write failure replaced: %v", err)
		}
	}
}

func TestEvalReportSeparatesAnswerableAndUnanswerableCases(t *testing.T) {
	results := []retrievalCaseResult{
		{Expected: []int64{1}, Retrieved: []int64{1}, Hit: true, Recall: 1, ReciprocalRank: 1},
		{Expected: []int64{1}, Retrieved: []int64{}},
		{Expected: []int64{}, Retrieved: []int64{}, NoAnswerExpected: true, Abstained: true},
		{Expected: []int64{}, Retrieved: []int64{1}, NoAnswerExpected: true},
	}
	var output bytes.Buffer
	err := writeEvalReport(&output, "embed", askOptions{}, evalReportOptions{Format: "json", MinAbstentionRate: .8}, results)
	if err == nil || !strings.Contains(err.Error(), "quality gate failed") {
		t.Fatalf("gate: %v", err)
	}
	var report evalReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.AnswerableCount != 2 || report.UnanswerableCount != 2 || report.Passed || len(report.Failures) != 1 {
		t.Fatalf("report: %+v", report)
	}
	for _, metric := range []*float64{report.Metrics.HitRate, report.Metrics.MeanRecall, report.Metrics.MRR, report.Metrics.AbstentionRate} {
		if metric == nil || *metric != .5 {
			t.Fatalf("unexpected metric: %v", metric)
		}
	}
}

func TestEvalReportMissingCategoryIsUnavailable(t *testing.T) {
	for _, negativeOnly := range []bool{false, true} {
		result := retrievalCaseResult{Expected: []int64{1}, Retrieved: []int64{1}, Hit: true, Recall: 1, ReciprocalRank: 1}
		missingKey := `"abstention_rate": null`
		if negativeOnly {
			result = retrievalCaseResult{Expected: []int64{}, Retrieved: []int64{}, NoAnswerExpected: true, Abstained: true}
			missingKey = `"hit_rate": null`
		}
		for _, format := range []string{"text", "json"} {
			var output bytes.Buffer
			if err := writeEvalReport(&output, "embed", askOptions{}, evalReportOptions{Format: format}, []retrievalCaseResult{result}); err != nil {
				t.Fatal(err)
			}
			want := missingKey
			if format == "text" {
				want = "N/A"
			}
			if !strings.Contains(output.String(), want) {
				t.Fatalf("missing %s: %s", want, output.String())
			}
		}
	}
}
