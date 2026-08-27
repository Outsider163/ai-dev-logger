package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
)

func TestSelectNotesForEmbedding(t *testing.T) {
	notes := []store.Note{
		{ID: 1, Title: "Current", Body: "same body"},
		{ID: 2, Title: "Stale", Body: "new body"},
		{ID: 3, Title: "Missing", Body: "body"},
	}
	embeddings := []store.NoteEmbedding{
		{
			NoteID:      1,
			Model:       "model-a",
			ContentHash: testContentHash(store.NoteEmbeddingText(notes[0])),
		},
		{
			NoteID:      2,
			Model:       "model-a",
			ContentHash: testContentHash("old text"),
		},
		{
			NoteID:      3,
			Model:       "model-b",
			ContentHash: testContentHash(store.NoteEmbeddingText(notes[2])),
		},
	}

	pending, skipped := selectNotesForEmbedding(notes, embeddings, "model-a", false)
	if skipped != 1 {
		t.Fatalf("expected 1 skipped note, got %d", skipped)
	}
	if len(pending) != 2 || pending[0].ID != 2 || pending[1].ID != 3 {
		t.Fatalf("expected stale and missing notes, got %#v", pending)
	}

	pending, skipped = selectNotesForEmbedding(notes, embeddings, "model-a", true)
	if skipped != 0 || len(pending) != len(notes) {
		t.Fatalf("expected force to select all notes, pending=%d skipped=%d", len(pending), skipped)
	}
}

func TestIncrementalEmbeddingSkipsUnchangedAPICall(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float64{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "Incremental", Body: "same body"})
	if err != nil {
		t.Fatal(err)
	}
	client := llm.NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "model-a",
	})

	pending, skipped := selectNotesForEmbedding([]store.Note{note}, nil, "model-a", false)
	if len(pending) != 1 || skipped != 0 {
		t.Fatalf("expected missing note to be pending, pending=%d skipped=%d", len(pending), skipped)
	}
	if err := saveNoteEmbedding(ctx, db, client, "model-a", pending[0]); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected 1 API request, got %d", got)
	}

	embeddings, err := db.ListEmbeddings(ctx, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	pending, skipped = selectNotesForEmbedding([]store.Note{note}, embeddings, "model-a", false)
	if len(pending) != 0 || skipped != 1 {
		t.Fatalf("expected unchanged note to be skipped, pending=%d skipped=%d", len(pending), skipped)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected unchanged note not to call API, got %d requests", got)
	}

	pending, skipped = selectNotesForEmbedding([]store.Note{note}, embeddings, "model-a", true)
	if len(pending) != 1 || skipped != 0 {
		t.Fatalf("expected force to select note, pending=%d skipped=%d", len(pending), skipped)
	}
	if err := saveNoteEmbedding(ctx, db, client, "model-a", pending[0]); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected force to make a second API request, got %d", got)
	}
}

func testContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
