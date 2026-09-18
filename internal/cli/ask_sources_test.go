package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
)

func TestAskSourcesBalanceNotesAndRespectBudget(t *testing.T) {
	matches := []semanticMatch{
		{note: store.Note{ID: 1, Body: strings.Repeat("一", 1200)}, chunkIndex: 0, score: 1},
		{note: store.Note{ID: 1, Body: strings.Repeat("二", 1200)}, chunkIndex: 1, score: .9},
		{note: store.Note{ID: 2, Body: strings.Repeat("三", 1200)}, chunkIndex: 0, score: .8},
		{note: store.Note{ID: 3, Body: "outside note limit"}, chunkIndex: 0, score: .7},
	}
	for _, tc := range []struct{ budget, perNote, want int }{{12000, 2, 3}, {3000, 2, 2}, {12000, 1, 2}} {
		sources, used := selectAskSources(matches, 2, tc.perNote, tc.budget)
		if len(sources) != tc.want || sources[0].ID != 1 || sources[1].ID != 2 {
			t.Fatalf("selection = %+v", sources)
		}
		actual := 0
		for _, source := range sources {
			actual += utf8.RuneCountInString(llm.KnowledgeSourceText(source))
		}
		if actual != used || used > tc.budget {
			t.Fatalf("budget: actual=%d used=%d limit=%d", actual, used, tc.budget)
		}
	}
}

func TestAskSourcesBoundMetadataAndDeduplicateChunks(t *testing.T) {
	match := semanticMatch{note: store.Note{ID: 1, Title: strings.Repeat("t", 9000), Body: "body", Summary: strings.Repeat("s", 9000), Tags: []string{strings.Repeat("tag", 9000)}}}
	sources, used := selectAskSources([]semanticMatch{match, match}, 1, 5, 2400)
	if len(sources) != 1 || used > 2400 {
		t.Fatalf("sources=%d size=%d", len(sources), used)
	}
}

func TestAskIncludesComplementaryChunksFromSameNote(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	cause, fix := "Cause: transaction held open.", "Fix: shorten the transaction."
	body := cause + strings.Repeat("a", 1200-len(cause)) + fix + strings.Repeat("b", 1200-len(fix)) + "Unrelated third section."
	dbPath := filepath.Join(t.TempDir(), "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "Database troubleshooting", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "embed", [][]float64{{1, 0}, {.9, .1}, {-1, 0}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
			return
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if len(request.Messages) != 2 {
			t.Error("missing prompt")
			w.WriteHeader(400)
			return
		}
		prompt := request.Messages[1].Content
		if !strings.Contains(prompt, cause) || !strings.Contains(prompt, fix) || strings.Contains(prompt, "Unrelated third section") {
			t.Error("complementary evidence missing or unrelated evidence included")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"事务持续过久，应缩短事务。[Note #1]"}}]}`)
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := appconfig.Save(configPath, appconfig.Config{Chat: appconfig.ProviderConfig{APIKey: "test", BaseURL: server.URL, Model: "chat"}, Embedding: appconfig.ProviderConfig{APIKey: "test", BaseURL: server.URL, Model: "embed"}}); err != nil {
		t.Fatal(err)
	}
	var output, warnings bytes.Buffer
	if err := runAsk(ctx, askOptions{ConfigPath: configPath, DBPath: dbPath, Question: "What caused the issue and how was it fixed?", Limit: 1, ChunksPerNote: 2, ContextChars: 12000, MinScore: .2, Output: &output, Error: &warnings}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "[Note #1 / Chunk 1]") || !strings.Contains(output.String(), "[Note #1 / Chunk 2]") || strings.Contains(output.String(), "Chunk 3") {
		t.Fatal(output.String())
	}
}

func TestAskRejectsInvalidContextSettingsBeforeOpeningDatabase(t *testing.T) {
	for _, tc := range []struct{ notes, chunks, budget int }{{21, 2, 12000}, {1, 0, 12000}, {1, 6, 12000}, {1, 2, 2399}, {1, 2, 48001}} {
		err := runAsk(context.Background(), askOptions{Question: "question", Limit: tc.notes, ChunksPerNote: tc.chunks, ContextChars: tc.budget})
		if err == nil || !strings.Contains(err.Error(), "must be between") {
			t.Fatalf("settings %+v: %v", tc, err)
		}
	}
}
