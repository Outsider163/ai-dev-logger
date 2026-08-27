package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appconfig "ai-dev-logger/internal/config"
)

func TestRunDoctorOfflineReady(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	databaseFile := filepath.Join(tempDir, "data", "notes.db")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey:         "doctor-secret-key",
		BaseURL:        "https://example.com/v1",
		Model:          "chat-model",
		EmbeddingModel: "embedding-model",
	})

	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile,
		DBPath:     databaseFile,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("expected ready offline doctor to succeed, got %v\n%s", err, output.String())
	}

	for _, expected := range []string{
		"[PASS] config",
		"[PASS] database",
		"[PASS] API key",
		"[PASS] LLM base URL",
		"[PASS] chat model",
		"[PASS] embedding model",
		"[WARN] online probes",
		"Summary: 6 passed, 1 warning(s), 0 failed, 0 skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected %q in output, got:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "doctor-secret-key") {
		t.Fatalf("doctor output must not reveal the API key: %s", output.String())
	}
}

func TestRunDoctorReportsMissingSettings(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	tempDir := t.TempDir()
	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: filepath.Join(tempDir, "missing-config.json"),
		DBPath:     filepath.Join(tempDir, "notes.db"),
		Timeout:    time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "3 failed check(s)") {
		t.Fatalf("expected three failed checks, got %v\n%s", err, output.String())
	}

	for _, expected := range []string{
		"[WARN] config",
		"[FAIL] API key",
		"[FAIL] chat model",
		"[FAIL] embedding model",
		"config set --api-key",
		"config set --model",
		"config set --embedding-model",
		"Summary: 2 passed, 2 warning(s), 3 failed, 0 skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected %q in output, got:\n%s", expected, output.String())
		}
	}
}

func TestRunDoctorSkipsConfigDependentChecksWhenConfigIsInvalid(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile,
		DBPath:     filepath.Join(tempDir, "notes.db"),
		Online:     true,
		Timeout:    time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "1 failed check(s)") {
		t.Fatalf("expected invalid config to fail, got %v\n%s", err, output.String())
	}

	for _, expected := range []string{
		"[FAIL] config",
		"[SKIP] API key",
		"[SKIP] LLM base URL",
		"[SKIP] chat model",
		"[SKIP] embedding model",
		"[SKIP] chat API",
		"[SKIP] embedding API",
		"Summary: 1 passed, 0 warning(s), 1 failed, 6 skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected %q in output, got:\n%s", expected, output.String())
		}
	}
}

func TestRunDoctorOnlineProbesChatAndEmbedding(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		switch r.URL.Path {
		case "/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"role": "assistant", "content": "OK"}},
				},
			})
		case "/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float64{0.1, 0.2, 0.3}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		Model:          "chat-model",
		EmbeddingModel: "embedding-model",
	})

	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile,
		DBPath:     filepath.Join(tempDir, "notes.db"),
		Online:     true,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("expected online doctor to succeed, got %v\n%s", err, output.String())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two online requests, got %d", requests.Load())
	}
	for _, expected := range []string{
		"[PASS] chat API",
		"[PASS] embedding API",
		"request succeeded (3 dimensions)",
		"Summary: 8 passed, 0 warning(s), 0 failed, 0 skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected %q in output, got:\n%s", expected, output.String())
		}
	}
}

func TestValidateHTTPBaseURL(t *testing.T) {
	for _, value := range []string{"", "example.com/v1", "file:///tmp/api", "://bad"} {
		if err := validateHTTPBaseURL(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
	for _, value := range []string{"https://api.openai.com/v1", "http://127.0.0.1:8080/v1"} {
		if err := validateHTTPBaseURL(value); err != nil {
			t.Fatalf("expected %q to be accepted, got %v", value, err)
		}
	}
}

func writeDoctorConfig(t *testing.T, path string, llmConfig appconfig.LLMConfig) {
	t.Helper()
	if err := appconfig.Save(path, appconfig.Config{LLM: llmConfig}); err != nil {
		t.Fatal(err)
	}
}
