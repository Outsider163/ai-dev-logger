package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appconfig "ai-dev-logger/internal/config"
)

func TestEnhanceNote(t *testing.T) {
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
