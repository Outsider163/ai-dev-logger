package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"ai-dev-logger/internal/llm"
)

func comparisonFixture(t *testing.T, name string, sources []llm.SearchNote) string {
	t.Helper()
	results := []retrievalCaseResult{
		scoreRetrieval(retrievalCase{Question: "known", ExpectedNoteIDs: []int64{1}}, sources),
		scoreRetrieval(retrievalCase{Question: "unknown", ExpectedNoteIDs: []int64{}}, nil),
	}
	var out bytes.Buffer
	if err := writeEvalReport(&out, "test-model", askOptions{Limit: 2, ChunksPerNote: 2, ContextChars: 12000, MinScore: .2}, evalReportOptions{Format: "json"}, results); err != nil {
		t.Fatal(err)
	}
	return writeIngestFixture(t, t.TempDir(), name, "\ufeff"+out.String())
}

func TestEvalCompareCommandIsOfflineAndShowsRegressions(t *testing.T) {
	before := comparisonFixture(t, "before.json", []llm.SearchNote{{ID: 1}})
	after := comparisonFixture(t, "after.json", []llm.SearchNote{{ID: 2}, {ID: 1}})
	command := newEvalCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"compare", before, after})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MRR: 1.0000 -> 0.5000 (delta -0.5000)", "Case 1 [REGRESSION]", "Regressed cases: 1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q: %s", want, output.String())
		}
	}
}

func TestEvalComparisonValidatesReports(t *testing.T) {
	path := comparisonFixture(t, "valid.json", []llm.SearchNote{{ID: 1}})
	valid, err := readEvalComparisonReport(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*evalReport)
	}{
		{"version", func(r *evalReport) { r.SchemaVersion = 1 }},
		{"count", func(r *evalReport) { r.CaseCount++ }},
		{"aggregate", func(r *evalReport) { r.Metrics.MRR = evalRatio(0, 1) }},
		{"score", func(r *evalReport) { r.Cases[0].Recall = 0 }},
		{"missing_ids", func(r *evalReport) { r.Cases[0].Expected = nil }},
		{"duplicate", func(r *evalReport) { r.Cases[0].Retrieved = []int64{1, 1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(valid)
			var modified evalReport
			if err := json.Unmarshal(data, &modified); err != nil {
				t.Fatal(err)
			}
			tc.change(&modified)
			data, _ = json.Marshal(modified)
			path := writeIngestFixture(t, t.TempDir(), "bad.json", string(data))
			if _, err := readEvalComparisonReport(path); err == nil {
				t.Fatal("accepted invalid report")
			}
		})
	}
	for _, body := range []string{"{}", "null", strings.Repeat(" ", (2<<20)+1), `{} {}`} {
		path := writeIngestFixture(t, t.TempDir(), "invalid.json", body)
		if _, err := readEvalComparisonReport(path); err == nil {
			t.Fatal("accepted malformed input")
		}
	}
}

func TestCompareRejectsDifferentCasesAndHandlesNoAnswer(t *testing.T) {
	path := comparisonFixture(t, "valid.json", nil)
	before, err := readEvalComparisonReport(path)
	if err != nil {
		t.Fatal(err)
	}
	after, err := readEvalComparisonReport(path)
	if err != nil {
		t.Fatal(err)
	}
	after.Cases[1] = scoreRetrieval(retrievalCase{Question: "unknown", ExpectedNoteIDs: []int64{}}, []llm.SearchNote{{ID: 2}})
	var out bytes.Buffer
	if err := compareEvalReports(&out, before, after); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Case 2 [REGRESSION]") {
		t.Fatal(out.String())
	}
	after.Cases[0].Question = "different"
	out.Reset()
	if err := compareEvalReports(&out, before, after); err == nil || out.Len() != 0 {
		t.Fatal("different cases accepted")
	}
	want := errors.New("write failed")
	if err := compareEvalReports(evalFailWriter{want}, before, before); !errors.Is(err, want) {
		t.Fatalf("writer: %v", err)
	}
}
