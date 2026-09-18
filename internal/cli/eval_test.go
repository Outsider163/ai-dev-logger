package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
)

func TestScoreRetrievalCountsNotesNotChunks(t *testing.T) {
	item := retrievalCase{Question: "q", ExpectedNoteIDs: []int64{2, 3}}
	result := scoreRetrieval(item, []llm.SearchNote{{ID: 1}, {ID: 1}, {ID: 2}, {ID: 2}})
	if !result.Hit || result.Recall != .5 || result.ReciprocalRank != .5 || len(result.Retrieved) != 2 {
		t.Fatalf("metrics: %+v", result)
	}
	miss := scoreRetrieval(item, nil)
	if miss.Hit || miss.Recall != 0 || miss.ReciprocalRank != 0 {
		t.Fatalf("miss: %+v", miss)
	}
}

func TestReadRetrievalCasesRejectsInvalidData(t *testing.T) {
	for _, body := range []string{
		`[]`, `null`, `[{"question":"","expected_note_ids":[1]}]`,
		`[{"question":"q"}]`, `[{"question":"q","expected_note_ids":null}]`, `[{"question":"q","expected_note_ids":[0]}]`,
		`[{"question":"q","expected_note_ids":[1,1]}]`, `[{"question":"q","expected_note_ids":[1],"extra":1}]`,
		`[{"question":"q","expected_note_ids":[1]}] []`,
	} {
		path := writeIngestFixture(t, t.TempDir(), "cases.json", body)
		if _, err := readRetrievalCases(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestRetrievalEvalUsesEmbeddingOnlyAndReportsMisses(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected API %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		switch request.Input {
		case "first":
			fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
		case "second":
			fmt.Fprint(w, `{"data":[{"embedding":[0,1]}]}`)
		default:
			fmt.Fprint(w, `{"data":[{"embedding":[-1,-1]}]}`)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i, vector := range [][]float64{{1, 0}, {0, 1}} {
		note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: fmt.Sprint(i), Body: "body"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{NoteID: note.ID, Model: "embed", Text: store.NoteEmbeddingText(note), Vector: vector}); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(dir, "config.json")
	if err := appconfig.Save(configPath, appconfig.Config{Embedding: appconfig.ProviderConfig{APIKey: "test", BaseURL: server.URL, Model: "embed"}}); err != nil {
		t.Fatal(err)
	}
	path := writeIngestFixture(t, dir, "cases.json", `[{"question":"first","expected_note_ids":[1]},{"question":"second","expected_note_ids":[2]},{"question":"miss","expected_note_ids":[1]}]`)
	var output, warnings bytes.Buffer
	options := askOptions{DBPath: dbPath, ConfigPath: configPath, Limit: 1, ChunksPerNote: 2, ContextChars: 12000, MinScore: .2, Output: &output, Error: &warnings}
	if err := runRetrievalEval(ctx, path, options, evalReportOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Hit rate: 0.6667", "Mean recall: 0.6667", "MRR: 0.6667", "expected: [1]; retrieved: []"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q: %s", expected, output.String())
		}
	}
	if requests.Load() != 3 {
		t.Fatalf("requests=%d", requests.Load())
	}
	output.Reset()
	if err := runRetrievalEval(ctx, path, options, evalReportOptions{Format: "json", MinHitRate: .8}); err == nil || !strings.Contains(err.Error(), "quality gate failed") {
		t.Fatalf("expected quality gate failure, got %v", err)
	}
	var report evalReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\n%s", err, output.String())
	}
	if report.Passed || report.CaseCount != 3 || report.Metrics.HitRate == nil || *report.Metrics.HitRate != 2.0/3 || len(report.Failures) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if requests.Load() != 6 {
		t.Fatalf("requests=%d", requests.Load())
	}
	if err := os.WriteFile(path, []byte(`[{"question":"first","expected_note_ids":[99]}]`), 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runRetrievalEval(ctx, path, options, evalReportOptions{}); err == nil || !strings.Contains(err.Error(), "expected note #99") {
		t.Fatalf("missing note error = %v", err)
	}
	if requests.Load() != 6 || output.Len() != 0 {
		t.Fatal("invalid expectations made API calls or produced a report")
	}
	path = writeIngestFixture(t, dir, "mixed.json", `[{"question":"first","expected_note_ids":[1]},{"question":"miss","expected_note_ids":[]},{"question":"second","expected_note_ids":[]}]`)
	if err := runRetrievalEval(ctx, path, options, evalReportOptions{Format: "json", MinAbstentionRate: 1}); err == nil || !strings.Contains(err.Error(), "quality gate failed") {
		t.Fatalf("gate: %v", err)
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 9 || report.Metrics.HitRate == nil || *report.Metrics.HitRate != 1 || report.Metrics.AbstentionRate == nil || *report.Metrics.AbstentionRate != .5 {
		t.Fatalf("mixed report: %+v; requests=%d", report, requests.Load())
	}
}

func TestNoAnswerScoring(t *testing.T) {
	item := retrievalCase{Question: "absent", ExpectedNoteIDs: []int64{}}
	for _, sources := range [][]llm.SearchNote{nil, {{ID: 1}}} {
		result := scoreRetrieval(item, sources)
		if !result.NoAnswerExpected || result.Abstained != (len(sources) == 0) || result.Hit || result.Recall != 0 || result.ReciprocalRank != 0 {
			t.Fatalf("no-answer score: %+v", result)
		}
	}
}

func TestEvalCoverageValidatedBeforeProvider(t *testing.T) {
	for _, tc := range []struct {
		body    string
		options evalReportOptions
		want    string
	}{
		{`[{"question":"q","expected_note_ids":[]}]`, evalReportOptions{MinHitRate: 1}, "answerable cases are required"},
		{`[{"question":"q","expected_note_ids":[1]}]`, evalReportOptions{MinAbstentionRate: 1}, "no-answer cases are required"},
	} {
		path := writeIngestFixture(t, t.TempDir(), "cases.json", tc.body)
		err := runRetrievalEval(context.Background(), path, askOptions{ConfigPath: "missing-config", Limit: 1, ChunksPerNote: 2, ContextChars: 12000}, tc.options)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("coverage: %v", err)
		}
	}
}
