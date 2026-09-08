package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
)

func TestLongNoteEmbeddingRetrievalAndAtomicFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "reference", Body: strings.Repeat("a", 1200) + "TARGET solution"})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad", 400)
			return
		}
		vector := []float64{1, 0}
		if strings.Contains(request.Input, "TARGET") {
			if fail.Load() {
				http.Error(w, "test failure", 400)
				return
			}
			vector = []float64{0, 1}
			if strings.Contains(request.Input, strings.Repeat("a", 100)) {
				t.Error("embedding included unrelated chunk")
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": vector}}})
	}))
	defer server.Close()
	client := llm.NewClient(appconfig.LLMConfig{APIKey: "test", BaseURL: server.URL, EmbeddingModel: "m"})
	if _, err := saveNoteEmbedding(ctx, db, client, "m", note); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("API calls=%d", calls.Load())
	}
	vectors, err := db.ListEmbeddings(ctx, "m")
	if err != nil {
		t.Fatal(err)
	}
	pending, skipped := selectNotesForEmbedding([]store.Note{note}, vectors, "m", false)
	if len(pending) != 0 || skipped != 1 {
		t.Fatal("complete chunk index not skipped")
	}
	pending, skipped = selectNotesForEmbedding([]store.Note{note}, vectors[:1], "m", false)
	if len(pending) != 1 || skipped != 0 {
		t.Fatal("incomplete index incorrectly skipped")
	}
	candidates, err := db.ListEmbeddedNotes(ctx, "m")
	if err != nil {
		t.Fatal(err)
	}
	current, stale := selectCurrentSemanticCandidates(candidates)
	if stale != 0 || len(current) != 2 {
		t.Fatal("chunk hashes mismatch")
	}
	matches, invalid := rankSemanticMatches([]float64{0, 1}, current, -1)
	if len(invalid) != 0 || len(matches) != 1 || matches[0].chunkIndex != 1 || matches[0].note.Body != "TARGET solution" {
		t.Fatalf("wrong best chunk: %#v", matches)
	}
	fail.Store(true)
	if _, err := saveNoteEmbedding(ctx, db, client, "m", note); err == nil {
		t.Fatal("expected API failure")
	}
	after, err := db.ListEmbeddings(ctx, "m")
	if err != nil || len(after) != 2 || !after[0].UpdatedAt.Equal(vectors[0].UpdatedAt) || !after[1].UpdatedAt.Equal(vectors[1].UpdatedAt) {
		t.Fatal("failed reindex changed saved vectors")
	}
}

func TestInteractiveShowsChunksWithoutSavingCommand(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "notes.db")
	var out bytes.Buffer
	input := strings.Repeat("中", 1300) + "\nshow 1 --chunks\n/list\nexit\n"
	if err := runInteractive(ctx, strings.NewReader(input), &out, path); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"note #1: 2 chunks", "[Note #1 / Chunk 1]", "[Note #1 / Chunk 2]"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	notes := readInteractiveTestNotes(t, path)
	if len(notes) != 1 {
		t.Fatal("show chunks became a note")
	}
}
