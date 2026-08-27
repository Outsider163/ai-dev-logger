package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	if _, err := saveNoteEmbedding(ctx, db, client, "model-a", pending[0]); err != nil {
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
	if _, err := saveNoteEmbedding(ctx, db, client, "model-a", pending[0]); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected force to make a second API request, got %d", got)
	}
}

func TestSaveEmbeddingBatchContinuesAfterNoteFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	notes := make([]store.Note, 0, 3)
	for _, title := range []string{"First note", "Fail note", "Third note"} {
		note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: title, Body: "body"})
		if err != nil {
			t.Fatal(err)
		}
		notes = append(notes, note)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if strings.Contains(request.Input, "Fail note") {
			http.Error(w, "invalid note content", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float64{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	client := llm.NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "model-a",
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, batchErr := saveEmbeddingBatch(ctx, db, client, "model-a", notes, &stdout, &stderr)
	if batchErr != nil {
		t.Fatalf("unexpected fatal batch error: %v", batchErr)
	}
	if result.Generated != 2 || len(result.Failures) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Failures[0].NoteID != notes[1].ID {
		t.Fatalf("expected note #%d to fail, got %#v", notes[1].ID, result.Failures)
	}
	if requests.Load() != 3 {
		t.Fatalf("expected all three notes to be attempted, got %d requests", requests.Load())
	}
	if !strings.Contains(stdout.String(), "[3/3] saved embedding") {
		t.Fatalf("expected progress to reach the third note, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "failed note #") {
		t.Fatalf("expected per-note failure output, got %q", stderr.String())
	}

	embeddings, err := db.ListEmbeddings(ctx, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected two successful embeddings, got %d", len(embeddings))
	}
	failureErr := result.failureError()
	if failureErr == nil || !strings.Contains(failureErr.Error(), fmt.Sprintf("#%d", notes[1].ID)) {
		t.Fatalf("expected failed note id in final error, got %v", failureErr)
	}
}

func TestSaveEmbeddingBatchStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	notes := make([]store.Note, 0, 2)
	for _, title := range []string{"First note", "Second note"} {
		note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: title, Body: "body"})
		if err != nil {
			t.Fatal(err)
		}
		notes = append(notes, note)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		cancel()
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := llm.NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "model-a",
	})
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result, batchErr := saveEmbeddingBatch(ctx, db, client, "model-a", notes, &stdout, &stderr)
	if batchErr == nil || !errors.Is(batchErr, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", batchErr)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one request before cancellation, got %d", requests.Load())
	}
	if result.Generated != 0 || len(result.Failures) != 1 {
		t.Fatalf("unexpected partial result: %#v", result)
	}
	if strings.Contains(stdout.String(), fmt.Sprintf("note #%d", notes[1].ID)) {
		t.Fatalf("second note should not be attempted after cancellation: %q", stdout.String())
	}
}

func testContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
