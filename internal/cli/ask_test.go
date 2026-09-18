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

func TestRunAskUsesSeparateProvidersAndCitesSources(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	var embeddingRequests atomic.Int32
	var chatRequests atomic.Int32
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embeddingRequests.Add(1)
		if r.URL.Path != "/embeddings" || r.Header.Get("Authorization") != "Bearer embedding-key" {
			t.Errorf("unexpected embedding request: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "embedding-model" {
			t.Errorf("embedding model = %q", request.Model)
		}
		fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
	}))
	defer embeddingServer.Close()
	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatRequests.Add(1)
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer chat-key" {
			t.Errorf("unexpected chat request: %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "chat-model" {
			t.Errorf("chat model = %q", request.Model)
		}
		if len(request.Messages) < 2 || !strings.Contains(request.Messages[1].Content, "[Note #1]") {
			t.Error("chat prompt did not include retrieved source")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"根据记录，应先检查锁冲突。[Note #1]"}}]}`)
	}))
	defer chatServer.Close()

	dbPath := filepath.Join(t.TempDir(), "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "SQLite 锁冲突", Body: "排查 SQLite 锁冲突时，先确认并发写入和事务持续时间。"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "无关笔记", Body: "Go map 初始化。"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []store.UpsertEmbeddingInput{
		{NoteID: note.ID, Model: "embedding-model", Text: store.ChunkEmbeddingText(note, note.Body), Vector: []float64{1, 0}},
		{NoteID: other.ID, Model: "embedding-model", Text: store.ChunkEmbeddingText(other, other.Body), Vector: []float64{-1, 0}},
	} {
		if _, err := db.UpsertEmbedding(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := appconfig.Save(configPath, appconfig.Config{
		Chat:      appconfig.ProviderConfig{APIKey: "chat-key", BaseURL: chatServer.URL, Model: "chat-model"},
		Embedding: appconfig.ProviderConfig{APIKey: "embedding-key", BaseURL: embeddingServer.URL, Model: "embedding-model"},
	}); err != nil {
		t.Fatal(err)
	}
	var output, errors bytes.Buffer
	err = runAsk(ctx, askOptions{ConfigPath: configPath, DBPath: dbPath, Question: "之前如何处理数据库锁？", Limit: 3, MinScore: 0.5, Output: &output, Error: &errors, ChunksPerNote: 2, ContextChars: 12000})
	if err != nil {
		t.Fatalf("runAsk: %v\n%s", err, errors.String())
	}
	for _, expected := range []string{"Sources:", "[Note #1 / Chunk 1] SQLite 锁冲突", "Answer:", "[Note #1]"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q:\n%s", expected, output.String())
		}
	}
	if embeddingRequests.Load() != 1 || chatRequests.Load() != 1 {
		t.Fatalf("requests: embedding=%d chat=%d", embeddingRequests.Load(), chatRequests.Load())
	}
}

func TestRunAskSkipsChatWhenNothingMatches(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	var chatRequests atomic.Int32
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
	}))
	defer embeddingServer.Close()
	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { chatRequests.Add(1) }))
	defer chatServer.Close()
	dbPath := filepath.Join(t.TempDir(), "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "other", Body: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{NoteID: note.ID, Model: "embedding-model", Text: store.ChunkEmbeddingText(note, note.Body), Vector: []float64{-1, 0}}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := appconfig.Save(configPath, appconfig.Config{
		Chat:      appconfig.ProviderConfig{APIKey: "chat", BaseURL: chatServer.URL, Model: "chat-model"},
		Embedding: appconfig.ProviderConfig{APIKey: "embedding", BaseURL: embeddingServer.URL, Model: "embedding-model"},
	}); err != nil {
		t.Fatal(err)
	}
	var output, errors bytes.Buffer
	if err := runAsk(ctx, askOptions{ConfigPath: configPath, DBPath: dbPath, Question: "question", Limit: 3, MinScore: 0.5, Output: &output, Error: &errors, ChunksPerNote: 2, ContextChars: 12000}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "未生成 AI 回答") || chatRequests.Load() != 0 {
		t.Fatalf("unexpected no-match behavior: %q, chat=%d", output.String(), chatRequests.Load())
	}
}

func TestRunAskRejectsAnswerWithoutValidCitation(t *testing.T) {
	clearAskProviderEnvironment(t)
	ctx := context.Background()
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`) }))
	defer embeddingServer.Close()
	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"没有引用的回答"}}]}`)
	}))
	defer chatServer.Close()
	dbPath := filepath.Join(t.TempDir(), "notes.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	note, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "note", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{NoteID: note.ID, Model: "embed", Text: store.ChunkEmbeddingText(note, note.Body), Vector: []float64{1, 0}}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := appconfig.Save(configPath, appconfig.Config{Chat: appconfig.ProviderConfig{APIKey: "chat", BaseURL: chatServer.URL, Model: "chat"}, Embedding: appconfig.ProviderConfig{APIKey: "embed", BaseURL: embeddingServer.URL, Model: "embed"}}); err != nil {
		t.Fatal(err)
	}
	var output, errors bytes.Buffer
	err = runAsk(ctx, askOptions{ConfigPath: configPath, DBPath: dbPath, Question: "question", Limit: 1, MinScore: 0, Output: &output, Error: &errors, ChunksPerNote: 2, ContextChars: 12000})
	if err == nil || !strings.Contains(err.Error(), "contains no [Note #ID] citation") {
		t.Fatalf("expected citation failure, got %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("uncited answer was printed: %q", output.String())
	}
}

func clearAskProviderEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(appconfig.EnvChatAPIKey, "")
	t.Setenv(appconfig.EnvEmbeddingAPIKey, "")
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
}
