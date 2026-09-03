package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefault(t *testing.T) {
	want := Config{LLM: LLMConfig{BaseURL: "https://api.openai.com/v1"}}
	if got := Default(); got != want {
		t.Fatalf("Default() = %#v, want %#v", got, want)
	}
	cfg := Default()
	cfg.LLM.APIKey = "test-key"
	if got := Default(); got != want {
		t.Fatalf("changing one config changed the defaults: %#v", got)
	}
}

func TestDefaultPath(t *testing.T) {
	directory := t.TempDir()
	var configDirectory string
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", directory)
		configDirectory = directory
	case "darwin":
		t.Setenv("HOME", directory)
		configDirectory = filepath.Join(directory, "Library", "Application Support")
	case "linux":
		t.Setenv("XDG_CONFIG_HOME", directory)
		configDirectory = directory
	default:
		t.Skip("platform-specific config path is not covered on this OS")
	}
	want := filepath.Join(configDirectory, "ai-dev-logger", "config.json")
	if got := DefaultPath(); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolving a path must not create a file: %v", err)
	}
}

func TestDefaultPathFallsBackWhenUserDirectoryIsUnavailable(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", "")
	case "darwin":
		t.Setenv("HOME", "")
	case "linux":
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
	default:
		t.Skip("platform-specific config path is not covered on this OS")
	}
	if got := DefaultPath(); got != "ai-dev-logger.json" {
		t.Fatalf("fallback path = %q", got)
	}
}

func TestLoadMissingFileReturnsDefaultsWithoutCreatingFiles(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-created")
	cfg, err := Load(filepath.Join(parent, "config.json"))
	if err != nil || cfg != Default() {
		t.Fatalf("Load(missing) = %#v, %v", cfg, err)
	}
	if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loading a missing config created its parent: %v", err)
	}
}

func TestLoad(t *testing.T) {
	full := Config{LLM: LLMConfig{
		APIKey: "test-key", BaseURL: "https://example.test/v1",
		Model: "test-chat", EmbeddingModel: "test-embedding",
	}}
	for _, test := range []struct {
		name string
		data string
		want Config
	}{
		{"empty_file", "", Default()},
		{"empty_object", `{}`, Default()},
		{"null", `null`, Default()},
		{"null_llm", `{"llm":null}`, Default()},
		{"empty_llm", `{"llm":{}}`, Default()},
		{"empty_base_url", `{"llm":{"base_url":""}}`, Default()},
		{"unknown_fields", `{"future":true,"llm":{"future":"ignored"}}`, Default()},
		{"partial", `{"llm":{"model":"test-chat"}}`, Config{LLM: LLMConfig{
			BaseURL: Default().LLM.BaseURL, Model: "test-chat",
		}}},
		{"full", `{"llm":{"api_key":"test-key","base_url":"https://example.test/v1","model":"test-chat","embedding_model":"test-embedding"}}`, full},
		{"trailing_whitespace", "{} \r\n\t", Default()},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := Load(path)
			if err != nil || got != test.want {
				t.Fatalf("Load() = %#v, %v; want %#v", got, err, test.want)
			}
			assertConfigFileBytes(t, path, []byte(test.data))
		})
	}
}

func TestLoadRejectsInvalidJSONWithoutChangingFile(t *testing.T) {
	for _, data := range []string{
		" \n\t", "{", `{"llm":`, `{"llm":"wrong-type"}`,
		`{"llm":{"api_key":123}}`, `[]`, `{} {}`, `{} trailing`,
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err == nil || cfg != (Config{}) {
				t.Fatalf("invalid JSON must return an error and no partial config: %#v, %v", cfg, err)
			}
			assertConfigFileBytes(t, path, []byte(data))
		})
	}
}

func TestLoadReportsReadErrors(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err == nil || cfg != (Config{}) {
		t.Fatalf("reading a directory must fail: %#v, %v", cfg, err)
	}
}

func TestLoadDoesNotMergeEnvironmentKeysIntoStoredConfig(t *testing.T) {
	t.Setenv(EnvAPIKey, "environment-only-key")
	t.Setenv(OpenAIAPIKeyEnv, "fallback-only-key")
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.LLM.APIKey = "stored-key"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != cfg {
		t.Fatalf("Load must return the stored key, not an environment override: %#v, %v", got, err)
	}
}

func TestSaveCreatesParentsAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "directory with spaces", "config.json")
	cfg := Config{LLM: LLMConfig{
		APIKey: "test-key", BaseURL: "https://example.test/v1",
		Model: "test-chat", EmbeddingModel: "test-embedding",
	}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !bytes.Contains(data, []byte("\n  \"llm\": {")) || !bytes.HasSuffix(data, []byte("}\n")) {
		t.Fatalf("expected indented JSON with a final newline: %q", data)
	}
	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil || decoded != cfg {
		t.Fatalf("saved JSON = %#v, %v; want %#v", decoded, err, cfg)
	}
	loaded, err := Load(path)
	if err != nil || loaded != cfg {
		t.Fatalf("round trip = %#v, %v; want %#v", loaded, err, cfg)
	}

	t.Run("owner_only_permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Unix permission bits do not describe Windows ACLs")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
		}
	})
}

func TestSaveReplacesPreviousContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	old := Default()
	old.LLM.APIKey = "long-previous-test-key"
	old.LLM.Model = "previous-chat-model"
	if err := Save(path, old); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got != Default() {
		t.Fatalf("replacement left stale configuration: %#v, %v", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(old.LLM.APIKey)) {
		t.Fatal("saved file still contains the removed key")
	}
}

func TestSaveReportsFilesystemErrors(t *testing.T) {
	t.Run("parent_is_file", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "parent")
		original := []byte("do not replace this file")
		if err := os.WriteFile(parent, original, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Save(filepath.Join(parent, "config.json"), Default()); err == nil {
			t.Fatal("expected parent creation error")
		}
		assertConfigFileBytes(t, parent, original)
	})
	t.Run("target_is_directory", func(t *testing.T) {
		path := t.TempDir()
		sentinel := filepath.Join(path, "keep.txt")
		original := []byte("keep")
		if err := os.WriteFile(sentinel, original, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Save(path, Default()); err == nil {
			t.Fatal("expected write error")
		}
		assertConfigFileBytes(t, sentinel, original)
	})
}

func TestMaskSecret(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", ""}, {"a", "********"}, {"12345678", "********"},
		{"123456789", "1234...6789"}, {"sk-test-secret-1234", "sk-t...1234"},
	} {
		t.Run(test.input, func(t *testing.T) {
			if got := MaskSecret(test.input); got != test.want {
				t.Fatalf("MaskSecret() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveAPIKeySkipsWhitespaceOnlyValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		app        string
		configured string
		openAI     string
		key        string
		source     string
	}{
		{"blank_app", " \t\n", " config-key ", "openai-key", "config-key", "config file"},
		{"blank_app_and_config", " \t", " \r\n", " openai-key ", "openai-key", OpenAIAPIKeyEnv},
		{"all_blank", " \t", " \r\n", " \n", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(EnvAPIKey, test.app)
			t.Setenv(OpenAIAPIKeyEnv, test.openAI)
			key, source := ResolveAPIKey(test.configured)
			if key != test.key || source != test.source {
				t.Fatalf("ResolveAPIKey() = %q, %q; want %q, %q", key, source, test.key, test.source)
			}
			if os.Getenv(EnvAPIKey) != test.app || os.Getenv(OpenAIAPIKeyEnv) != test.openAI {
				t.Fatal("resolving a key must not change environment variables")
			}
		})
	}
}

func assertConfigFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("config file contents changed unexpectedly")
	}
}

func TestResolveAPIKeyPriority(t *testing.T) {
	t.Setenv(EnvAPIKey, " app-key ")
	t.Setenv(OpenAIAPIKeyEnv, "openai-key")

	key, source := ResolveAPIKey("config-key")
	if key != "app-key" {
		t.Fatalf("expected app environment key, got %q", key)
	}
	if source != EnvAPIKey {
		t.Fatalf("expected source %s, got %q", EnvAPIKey, source)
	}
}

func TestResolveAPIKeyFallsBackToConfig(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	t.Setenv(OpenAIAPIKeyEnv, "openai-key")

	key, source := ResolveAPIKey(" config-key ")
	if key != "config-key" || source != "config file" {
		t.Fatalf("expected config key, got key=%q source=%q", key, source)
	}
}

func TestResolveAPIKeyFallsBackToOpenAIEnvironment(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	t.Setenv(OpenAIAPIKeyEnv, " openai-key ")

	key, source := ResolveAPIKey("")
	if key != "openai-key" || source != OpenAIAPIKeyEnv {
		t.Fatalf("expected OpenAI environment key, got key=%q source=%q", key, source)
	}
}

func TestResolveAPIKeyCanBeEmpty(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	t.Setenv(OpenAIAPIKeyEnv, "")

	key, source := ResolveAPIKey("")
	if key != "" || source != "" {
		t.Fatalf("expected empty result, got key=%q source=%q", key, source)
	}
}
