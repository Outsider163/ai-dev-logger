package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appconfig "ai-dev-logger/internal/config"
)

func TestEnhanceNote(t *testing.T) {
	clearAPIKeyEnvironment(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role": "assistant",
						"content": `{
							"title": "Go map concurrency",
							"body": "Use mutex or sync.Map.",
							"summary": "说明 Go map 并发读写的处理方式。",
							"tags": ["Go", "concurrency", "go"]
						}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	enhanced, err := client.EnhanceNote(context.Background(), EnhanceNoteInput{
		Title: "map panic",
		Body:  "map concurrent read write panic",
		Tags:  []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if enhanced.Title != "Go map concurrency" {
		t.Fatalf("unexpected title: %s", enhanced.Title)
	}
	if enhanced.Summary == "" {
		t.Fatal("expected summary")
	}
	if got := len(enhanced.Tags); got != 2 {
		t.Fatalf("expected deduplicated tags, got %d: %#v", got, enhanced.Tags)
	}
}

func TestEnhanceNoteRequiresConfig(t *testing.T) {
	client := NewClient(appconfig.LLMConfig{})

	if _, err := client.EnhanceNote(context.Background(), EnhanceNoteInput{}); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestCreateEmbedding(t *testing.T) {
	clearAPIKeyEnvironment(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "embedding-test-model" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		if req.Input != "hello embedding" {
			t.Fatalf("unexpected input: %s", req.Input)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float64{0.1, 0.2, 0.3}},
			},
		})
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})

	vector, err := client.CreateEmbedding(context.Background(), "hello embedding")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vector))
	}
	if vector[2] != 0.3 {
		t.Fatalf("unexpected vector: %#v", vector)
	}
}

func TestCreateEmbeddingRequiresConfig(t *testing.T) {
	client := NewClient(appconfig.LLMConfig{})

	if _, err := client.CreateEmbedding(context.Background(), "hello"); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestExplainSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, "[Note #7]") {
			t.Fatalf("expected note id in prompt, got %#v", req.Messages)
		}
		if !strings.Contains(req.Messages[1].Content, "Similarity: 0.9123") {
			t.Fatalf("expected similarity in prompt, got %q", req.Messages[1].Content)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]string{"role": "assistant", "content": "Use a mutex around the shared map. [Note #7]"},
			}},
		})
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{APIKey: "test-key", BaseURL: server.URL, Model: "test-model"})
	explanation, err := client.ExplainSearch(context.Background(), "How should I share a map?", []SearchNote{{ID: 7, Score: 0.9123, Title: "Go map", Body: "Use sync.Mutex."}})
	if err != nil {
		t.Fatal(err)
	}
	if explanation != "Use a mutex around the shared map. [Note #7]" {
		t.Fatalf("unexpected explanation: %s", explanation)
	}
}

func TestCreateEmbeddingUsesEnvironmentAPIKey(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "environment-key")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer environment-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float64{0.1, 0.2}}},
		})
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})
	if _, err := client.CreateEmbedding(context.Background(), "environment key"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateEmbeddingRetriesTransientStatuses(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		switch attempts {
		case 1:
			w.Header().Set("Retry-After", "0")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		case 2:
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float64{0.3, 0.4}}},
			})
		}
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})
	client.retryBaseDelay = 0
	client.maxRetryDelay = 0

	vector, err := client.CreateEmbedding(context.Background(), "retry test")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if len(vector) != 2 || vector[1] != 0.4 {
		t.Fatalf("unexpected vector: %#v", vector)
	}
}

func TestCreateEmbeddingDoesNotRetryClientError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "invalid request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})
	client.retryBaseDelay = 0

	_, err := client.CreateEmbedding(context.Background(), "bad request")
	if err == nil {
		t.Fatal("expected request error")
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "status 400") || !strings.Contains(err.Error(), "invalid request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrorResponseBodyIsLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", maxErrorResponseBytes+100) + "secret-tail"))
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})
	_, err := client.CreateEmbedding(context.Background(), "large error")
	if err == nil {
		t.Fatal("expected request error")
	}
	if !strings.Contains(err.Error(), "truncated to 4096 bytes") {
		t.Fatalf("expected truncation marker, got: %v", err)
	}
	if strings.Contains(err.Error(), "secret-tail") {
		t.Fatalf("error contains data beyond the response limit: %v", err)
	}
}

func TestRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		cancel()
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "embedding-test-model",
	})
	_, err := client.CreateEmbedding(ctx, "cancel retry")
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt before cancellation, got %d", attempts)
	}
}

func TestRetryDelayUsesRetryAfterAndCap(t *testing.T) {
	client := NewClient(appconfig.LLMConfig{})

	if got := client.retryDelay(0, "3"); got != 3*time.Second {
		t.Fatalf("expected Retry-After delay, got %s", got)
	}
	if got := client.retryDelay(5, "20"); got != defaultMaxRetryDelay {
		t.Fatalf("expected capped delay %s, got %s", defaultMaxRetryDelay, got)
	}
}

func clearAPIKeyEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
}
