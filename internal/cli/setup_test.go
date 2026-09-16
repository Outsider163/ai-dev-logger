package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	appconfig "ai-dev-logger/internal/config"
)

func TestRunSetupTestsThenSavesDeepSeekConfig(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.URL.Path != "/chat/completions" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q", got)
		}

		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "test-chat-model" {
			t.Errorf("model = %q", body.Model)
		}

		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	input := strings.NewReader("test-key\n" + server.URL + "\ntest-chat-model\n")
	var output bytes.Buffer

	err := runSetup(context.Background(), setupOptions{
		ConfigPath: configPath,
		Input:      input,
		Output:     &output,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("runSetup returned error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	want := appconfig.ProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-chat-model",
	}
	if !reflect.DeepEqual(cfg.Chat, want) {
		t.Fatalf("saved chat config = %#v, want %#v", cfg.Chat, want)
	}
	if !strings.Contains(output.String(), "connection test: passed") {
		t.Fatalf("unexpected output:\n%s", output.String())
	}
	for _, expected := range []string{"本地配置文件", "API 用量", "下一步: adl add --ai"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("setup guidance is missing %q:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "test-key") {
		t.Fatalf("setup guidance must not reveal the API key: %s", output.String())
	}
}

func TestRunSetupFailureDoesNotOverwriteConfig(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	original := appconfig.Config{LLM: appconfig.LLMConfig{
		APIKey:         "old-key",
		BaseURL:        "https://old.example/v1",
		Model:          "old-chat-model",
		EmbeddingModel: "old-embedding-model",
	}}
	if err := appconfig.Save(configPath, original); err != nil {
		t.Fatalf("save original config: %v", err)
	}

	input := strings.NewReader("new-key\n" + server.URL + "\nnew-chat-model\n")
	err := runSetup(context.Background(), setupOptions{
		ConfigPath: configPath,
		Input:      input,
		Output:     &bytes.Buffer{},
		Timeout:    time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "configuration was not saved") {
		t.Fatalf("expected unsaved test failure, got %v", err)
	}

	after, err := appconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load config after failure: %v", err)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatalf("config changed after failed test: got %#v, want %#v", after, original)
	}
}

func TestRunSetupUsesDeepSeekDefaultsAndPreservesEmbeddingProfile(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	configPath := filepath.Join(t.TempDir(), "config.json")
	original := appconfig.Config{LLM: appconfig.LLMConfig{
		EmbeddingModel: "stale-embedding-model",
	}}
	if err := appconfig.Save(configPath, original); err != nil {
		t.Fatalf("save original config: %v", err)
	}

	var output bytes.Buffer
	err := runSetup(context.Background(), setupOptions{
		ConfigPath: configPath,
		Input:      strings.NewReader("test-key\n\n\n"),
		Output:     &output,
		SkipTest:   true,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("runSetup returned error: %v", err)
	}

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if cfg.Chat.BaseURL != deepSeekBaseURL || cfg.Chat.Model != deepSeekDefaultModel {
		t.Fatalf("unexpected chat defaults: %#v", cfg.Chat)
	}
	if cfg.Embedding.Model != "stale-embedding-model" {
		t.Fatalf("embedding model changed: %q", cfg.Embedding.Model)
	}
	if !strings.Contains(output.String(), "connection test: skipped") {
		t.Fatalf("unexpected output:\n%s", output.String())
	}
}
