package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		"[SKIP] online probes",
		"Summary: 6 passed, 0 warning(s), 0 failed, 1 skipped",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("expected %q in output, got:\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "doctor-secret-key") {
		t.Fatalf("doctor output must not reveal the API key: %s", output.String())
	}
	assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
	assertDoctorCapability(t, output.String(), doctorPass, "AI enhancement", "optional; configured; online not checked")
	assertDoctorCapability(t, output.String(), doctorPass, "semantic search", "optional; configured; online not checked")
}

func TestRunDoctorAllowsLocalOnlyUse(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	for _, online := range []bool{false, true} {
		t.Run(fmt.Sprintf("online=%t", online), func(t *testing.T) {
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "missing-config.json")
			var output bytes.Buffer
			err := runDoctor(context.Background(), &output, doctorOptions{
				ConfigPath: configFile,
				DBPath:     filepath.Join(tempDir, "notes.db"),
				Online:     online,
				Timeout:    time.Second,
			})
			if err != nil {
				t.Fatalf("missing optional settings must not fail local-only use: %v\n%s", err, output.String())
			}

			assertDoctorOutput(t, output.String(),
				"[WARN] config",
				"[WARN] API key",
				"[WARN] chat model",
				"[WARN] embedding model",
				"config set --api-key",
				"config set --model",
				"config set --embedding-model",
			)
			if online {
				assertDoctorOutput(t, output.String(),
					"[SKIP] chat API", "[SKIP] embedding API",
					"Summary: 2 passed, 4 warning(s), 0 failed, 2 skipped",
				)
			} else {
				assertDoctorOutput(t, output.String(), "Summary: 2 passed, 4 warning(s), 0 failed, 1 skipped")
			}
			assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
			assertDoctorCapability(t, output.String(), doctorWarn, "AI enhancement", "optional; not configured; missing API key, chat model")
			assertDoctorCapability(t, output.String(), doctorWarn, "semantic search", "optional; not configured; missing API key, embedding model")
			if _, err := os.Stat(configFile); !os.IsNotExist(err) {
				t.Fatalf("doctor must not create the missing config file, got %v", err)
			}
		})
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
	assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
	assertDoctorCapability(t, output.String(), doctorFail, "AI enhancement", "optional; unavailable; configuration could not be loaded")
	assertDoctorCapability(t, output.String(), doctorFail, "semantic search", "optional; unavailable; configuration could not be loaded")
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

		writeDoctorProbeResponse(w, r)
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
	assertDoctorCapability(t, output.String(), doctorPass, "AI enhancement", "optional; configured; API reachable")
	assertDoctorCapability(t, output.String(), doctorPass, "semantic search", "optional; configured; API reachable")
}

func TestRunDoctorOnlyProbesConfiguredServices(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")

	for _, test := range []struct {
		name              string
		apiKey            string
		chatModel         string
		embeddingModel    string
		online            bool
		wantChatRequests  int32
		wantEmbedRequests int32
		wantAIStatus      doctorStatus
		wantSearchStatus  doctorStatus
		wantSummary       string
	}{
		{
			name: "offline", apiKey: "test-key", chatModel: "chat", embeddingModel: "embedding",
			wantAIStatus: doctorPass, wantSearchStatus: doctorPass,
			wantSummary: "Summary: 6 passed, 0 warning(s), 0 failed, 1 skipped",
		},
		{
			name: "chat_only", apiKey: "test-key", chatModel: "chat", embeddingModel: "  ", online: true,
			wantChatRequests: 1, wantAIStatus: doctorPass, wantSearchStatus: doctorWarn,
			wantSummary: "Summary: 6 passed, 1 warning(s), 0 failed, 1 skipped",
		},
		{
			name: "embedding_only", apiKey: "test-key", chatModel: "  ", embeddingModel: "embedding", online: true,
			wantEmbedRequests: 1, wantAIStatus: doctorWarn, wantSearchStatus: doctorPass,
			wantSummary: "Summary: 6 passed, 1 warning(s), 0 failed, 1 skipped",
		},
		{
			name: "no_models", apiKey: "test-key", online: true,
			wantAIStatus: doctorWarn, wantSearchStatus: doctorWarn,
			wantSummary: "Summary: 4 passed, 2 warning(s), 0 failed, 2 skipped",
		},
		{
			name: "no_api_key", apiKey: "  ", chatModel: "chat", embeddingModel: "embedding", online: true,
			wantAIStatus: doctorWarn, wantSearchStatus: doctorWarn,
			wantSummary: "Summary: 5 passed, 1 warning(s), 0 failed, 2 skipped",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var chatRequests, embedRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
				}
				switch r.URL.Path {
				case "/chat/completions":
					chatRequests.Add(1)
				case "/embeddings":
					embedRequests.Add(1)
				}
				writeDoctorProbeResponse(w, r)
			}))
			defer server.Close()

			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "config.json")
			writeDoctorConfig(t, configFile, appconfig.LLMConfig{
				APIKey: test.apiKey, BaseURL: server.URL,
				Model: test.chatModel, EmbeddingModel: test.embeddingModel,
			})
			var output bytes.Buffer
			err := runDoctor(context.Background(), &output, doctorOptions{
				ConfigPath: configFile, DBPath: filepath.Join(tempDir, "notes.db"),
				Online: test.online, Timeout: time.Second,
			})
			if err != nil {
				t.Fatalf("expected unconfigured services to be optional, got %v\n%s", err, output.String())
			}
			if chatRequests.Load() != test.wantChatRequests || embedRequests.Load() != test.wantEmbedRequests {
				t.Fatalf("unexpected probe counts: chat=%d, embedding=%d", chatRequests.Load(), embedRequests.Load())
			}
			assertDoctorOutput(t, output.String(), test.wantSummary)
			assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
			assertDoctorCapability(t, output.String(), test.wantAIStatus, "AI enhancement", "optional;")
			assertDoctorCapability(t, output.String(), test.wantSearchStatus, "semantic search", "optional;")
		})
	}
}

func TestRunDoctorReportsInvalidBaseURL(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey: "test-key", BaseURL: "not-a-url", Model: "chat", EmbeddingModel: "embedding",
	})
	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile, DBPath: filepath.Join(tempDir, "notes.db"),
		Online: true, Timeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "1 failed check(s)") {
		t.Fatalf("invalid settings must still fail, got %v\n%s", err, output.String())
	}
	assertDoctorOutput(t, output.String(),
		"[FAIL] LLM base URL", "[SKIP] chat API", "[SKIP] embedding API",
		"Summary: 5 passed, 0 warning(s), 1 failed, 2 skipped",
	)
	assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
	assertDoctorCapability(t, output.String(), doctorFail, "AI enhancement", "optional; unavailable; LLM base URL is invalid")
	assertDoctorCapability(t, output.String(), doctorFail, "semantic search", "optional; unavailable; LLM base URL is invalid")
}

func TestRunDoctorReportsDatabaseFailure(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey: "test-key", BaseURL: "https://example.com/v1", Model: "chat", EmbeddingModel: "embedding",
	})
	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile, DBPath: tempDir, Timeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "1 failed check(s)") {
		t.Fatalf("database failure must produce one failed check, got %v\n%s", err, output.String())
	}
	assertDoctorOutput(t, output.String(),
		"[FAIL] database", "Summary: 5 passed, 0 warning(s), 1 failed, 1 skipped",
	)
	assertDoctorCapability(t, output.String(), doctorFail, "local notes", "required; unavailable; database is not ready")
	assertDoctorCapability(t, output.String(), doctorFail, "AI enhancement", "optional; unavailable; database is not ready")
	assertDoctorCapability(t, output.String(), doctorFail, "semantic search", "optional; unavailable; database is not ready")
}

func TestRunDoctorReportsConfiguredAPIFailures(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	for _, test := range []struct {
		name             string
		path             string
		failedCheck      string
		failedCapability string
		readyCapability  string
	}{
		{"chat", "/chat/completions", "chat API", "AI enhancement", "semantic search"},
		{"embedding", "/embeddings", "embedding API", "semantic search", "AI enhancement"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path == test.path {
					http.Error(w, "invalid credentials", http.StatusUnauthorized)
					return
				}
				writeDoctorProbeResponse(w, r)
			}))
			defer server.Close()
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "config.json")
			writeDoctorConfig(t, configFile, appconfig.LLMConfig{
				APIKey: "test-key", BaseURL: server.URL, Model: "chat", EmbeddingModel: "embedding",
			})
			var output bytes.Buffer
			err := runDoctor(context.Background(), &output, doctorOptions{
				ConfigPath: configFile, DBPath: filepath.Join(tempDir, "notes.db"),
				Online: true, Timeout: time.Second,
			})
			if err == nil || !strings.Contains(err.Error(), "1 failed check(s)") {
				t.Fatalf("configured API failure must still fail, got %v\n%s", err, output.String())
			}
			if requests.Load() != 2 {
				t.Fatalf("one failed API must not prevent the other probe, got %d requests", requests.Load())
			}
			assertDoctorOutput(t, output.String(),
				"[FAIL] "+test.failedCheck, "status 401",
				"Summary: 7 passed, 0 warning(s), 1 failed, 0 skipped",
			)
			assertDoctorCapability(t, output.String(), doctorPass, "local notes", "required; ready")
			assertDoctorCapability(t, output.String(), doctorFail, test.failedCapability, "optional; unavailable; API check failed")
			assertDoctorCapability(t, output.String(), doctorPass, test.readyCapability, "optional; configured; API reachable")
		})
	}
}

func TestRunDoctorReportsOnlineTimeout(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.Close()
	}()
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey: "test-key", BaseURL: server.URL, Model: "chat",
	})
	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile, DBPath: filepath.Join(tempDir, "notes.db"),
		Online: true, Timeout: 30 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "1 failed check(s)") {
		t.Fatalf("online timeout must fail, got %v\n%s", err, output.String())
	}
	assertDoctorOutput(t, output.String(),
		"[FAIL] chat API", "context deadline exceeded",
		"Summary: 5 passed, 1 warning(s), 1 failed, 1 skipped",
	)
	assertDoctorCapability(t, output.String(), doctorFail, "AI enhancement", "optional; unavailable; API check failed")
}

func TestRunDoctorUsesEnvironmentAPIKey(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "environment-secret")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "fallback-secret")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer environment-secret" {
			t.Errorf("doctor used the wrong API key source")
		}
		writeDoctorProbeResponse(w, r)
	}))
	defer server.Close()
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.json")
	writeDoctorConfig(t, configFile, appconfig.LLMConfig{
		APIKey: "configured-secret", BaseURL: server.URL, Model: "chat",
	})
	var output bytes.Buffer
	err := runDoctor(context.Background(), &output, doctorOptions{
		ConfigPath: configFile, DBPath: filepath.Join(tempDir, "notes.db"),
		Online: true, Timeout: time.Second,
	})
	if err != nil || requests.Load() != 1 {
		t.Fatalf("expected one successful chat probe with the environment key, got %v and %d requests\n%s", err, requests.Load(), output.String())
	}
	assertDoctorOutput(t, output.String(), "configured via "+appconfig.EnvAPIKey)
	for _, secret := range []string{"environment-secret", "configured-secret", "fallback-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("doctor output must not reveal any API key: %s", output.String())
		}
	}
}

func TestRunDoctorRejectsInvalidTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			tempDir := t.TempDir()
			databaseFile := filepath.Join(tempDir, "notes.db")
			var output bytes.Buffer
			err := runDoctor(context.Background(), &output, doctorOptions{
				ConfigPath: filepath.Join(tempDir, "config.json"), DBPath: databaseFile,
				Timeout: timeout,
			})
			if err == nil || !strings.Contains(err.Error(), "timeout must be greater than zero") {
				t.Fatalf("expected invalid timeout to fail, got %v", err)
			}
			if _, err := os.Stat(databaseFile); !os.IsNotExist(err) {
				t.Fatalf("invalid timeout must not initialize the database, got %v", err)
			}
		})
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

func writeDoctorProbeResponse(w http.ResponseWriter, r *http.Request) {
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
}

func assertDoctorOutput(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Fatalf("expected %q in output, got:\n%s", value, output)
		}
	}
}

func assertDoctorCapability(t *testing.T, output string, status doctorStatus, name, detail string) {
	t.Helper()
	_, capabilities, found := strings.Cut(output, "\nCapabilities:\n")
	if !found {
		t.Fatalf("expected a capability summary, got:\n%s", output)
	}
	assertDoctorOutput(t, capabilities, fmt.Sprintf("[%-4s] %-16s %s", status, name, detail))
}
