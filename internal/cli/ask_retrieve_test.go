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
	"sync/atomic"
	"testing"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/store"
)

func TestRetrieveOnlyNeedsNoChatAndMatchesAnswerContext(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	var chatCalls, embeddingCalls atomic.Int32
	captured := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/embeddings":
			embeddingCalls.Add(1)
			fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
		case "/chat/completions":
			chatCalls.Add(1)
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
				t.Error("missing messages")
				w.WriteHeader(400)
				return
			}
			_, rest, ok := strings.Cut(request.Messages[1].Content, "<retrieved_notes>\n")
			if !ok {
				t.Error("missing retrieved context")
				w.WriteHeader(400)
				return
			}
			capturedContext, _, _ := strings.Cut(rest, "\n</retrieved_notes>")
			captured <- capturedContext
			fmt.Fprint(w, `{"choices":[{"message":{"content":"根据资料缩短事务。[Note #1]"}}]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	dbPath := filepath.Join(t.TempDir(), "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "SQLite", Body: "缩短事务可以减少锁等待。", Tags: []string{"sqlite"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{NoteID: note.ID, Model: "embed", Text: store.NoteEmbeddingText(note), Vector: []float64{1, 0}}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := appconfig.Config{Embedding: appconfig.ProviderConfig{APIKey: "embedding-only", BaseURL: server.URL, Model: "embed"}}
	if err := appconfig.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	var output, warnings bytes.Buffer
	options := askOptions{ConfigPath: configPath, DBPath: dbPath, Question: "如何减少锁等待？", Limit: 3, ChunksPerNote: 2, ContextChars: 12000, MinScore: .2, RetrieveOnly: true, Output: &output, Error: &warnings}
	if err := runAsk(ctx, options); err != nil {
		t.Fatal(err)
	}
	if chatCalls.Load() != 0 || embeddingCalls.Load() != 1 {
		t.Fatalf("calls: chat=%d embedding=%d", chatCalls.Load(), embeddingCalls.Load())
	}
	_, preview, ok := strings.Cut(output.String(), "Retrieved context (chat API not called):\n")
	if !ok || !strings.Contains(preview, note.Body) || strings.Contains(output.String(), "Answer:") {
		t.Fatal(output.String())
	}
	cfg.Chat = appconfig.ProviderConfig{APIKey: "chat", BaseURL: server.URL, Model: "chat"}
	if err := appconfig.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := runAsk(ctx, options); err != nil {
		t.Fatal(err)
	}
	if chatCalls.Load() != 0 {
		t.Fatal("retrieve-only called configured chat service")
	}
	options.RetrieveOnly = false
	output.Reset()
	if err := runAsk(ctx, options); err != nil {
		t.Fatal(err)
	}
	select {
	case actual := <-captured:
		if actual != preview {
			t.Fatalf("preview differs from actual model context:\npreview: %q\nactual: %q", preview, actual)
		}
	default:
		t.Fatal("chat context was not captured")
	}
	if chatCalls.Load() != 1 || embeddingCalls.Load() != 3 {
		t.Fatalf("unexpected request counts: chat=%d embedding=%d", chatCalls.Load(), embeddingCalls.Load())
	}
}
