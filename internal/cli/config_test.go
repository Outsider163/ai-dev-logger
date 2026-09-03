package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appconfig "ai-dev-logger/internal/config"

	"github.com/spf13/cobra"
)

func TestConfigPathAndShowDoNotCreateMissingConfig(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	for _, name := range []string{"path", "show"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "not-created")
			path := filepath.Join(parent, "config.json")
			output, err := executeConfigCommand(path, name)
			if err != nil {
				t.Fatal(err)
			}
			want := path + "\n"
			if name == "show" {
				want = fmt.Sprintf("path: %s\nllm.api_key: (empty)\nllm.api_key_source: (empty)\nllm.base_url: https://api.openai.com/v1\nllm.model: (empty)\nllm.embedding_model: (empty)\n", path)
			}
			if output != want {
				t.Fatalf("output = %q, want %q", output, want)
			}
			if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("read-only config command created files: %v", err)
			}
		})
	}
}

func TestConfigShowMasksEffectiveKeyUnlessRevealed(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured string
		app        string
		openAI     string
		reveal     bool
		wantKey    string
		wantSource string
	}{
		{"config", "cfg-secret-1234", "", "fallback-secret-5678", false, "cfg-...1234", "config file"},
		{"app_environment", "cfg-secret-1234", " app-secret-5678 ", "fallback-secret-9012", false, "app-...5678", appconfig.EnvAPIKey},
		{"fallback_environment", " \t", " \n", " fallback-secret-9012 ", false, "fall...9012", appconfig.OpenAIAPIKeyEnv},
		{"short_key", "short", "", "", false, "********", "config file"},
		{"empty", "", "", "", false, "(empty)", "(empty)"},
		{"reveal_config", "cfg-secret-1234", "", "", true, "cfg-secret-1234", "config file"},
		{"reveal_app", "cfg-secret-1234", " app-secret-5678 ", "", true, "app-secret-5678", appconfig.EnvAPIKey},
		{"reveal_empty", "", "", "", true, "(empty)", "(empty)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(appconfig.EnvAPIKey, test.app)
			t.Setenv(appconfig.OpenAIAPIKeyEnv, test.openAI)
			path := filepath.Join(t.TempDir(), "config.json")
			cfg := configCommandFixture()
			cfg.LLM.APIKey = test.configured
			if err := appconfig.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before := readConfigCommandFile(t, path)
			args := []string{"show"}
			if test.reveal {
				args = append(args, "--reveal")
			}
			output, err := executeConfigCommand(path, args...)
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("path: %s\nllm.api_key: %s\nllm.api_key_source: %s\nllm.base_url: %s\nllm.model: %s\nllm.embedding_model: %s\n",
				path, test.wantKey, test.wantSource, cfg.LLM.BaseURL, cfg.LLM.Model, cfg.LLM.EmbeddingModel)
			if output != want {
				t.Fatalf("unexpected config output:\n%s\nwant:\n%s", output, want)
			}
			if !bytes.Equal(readConfigCommandFile(t, path), before) {
				t.Fatal("show changed the config file")
			}
		})
	}
}

func TestConfigSetOnlyUpdatesExplicitFields(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want appconfig.LLMConfig
	}{
		{"key", []string{"--api-key", " new-test-key "}, appconfig.LLMConfig{
			APIKey: "new-test-key", BaseURL: "https://old.example/v1", Model: "old-chat", EmbeddingModel: "old-embedding",
		}},
		{"base_url", []string{"--base-url", " https://new.example/v1/// "}, appconfig.LLMConfig{
			APIKey: "old-test-key", BaseURL: "https://new.example/v1", Model: "old-chat", EmbeddingModel: "old-embedding",
		}},
		{"model", []string{"--model", " new-chat "}, appconfig.LLMConfig{
			APIKey: "old-test-key", BaseURL: "https://old.example/v1", Model: "new-chat", EmbeddingModel: "old-embedding",
		}},
		{"embedding", []string{"--embedding-model", " new-embedding "}, appconfig.LLMConfig{
			APIKey: "old-test-key", BaseURL: "https://old.example/v1", Model: "old-chat", EmbeddingModel: "new-embedding",
		}},
		{"all_fields", []string{"--api-key", " new-test-key ", "--base-url", " https://new.example/v1/ ", "--model", " new-chat ", "--embedding-model", " new-embedding "}, appconfig.LLMConfig{
			APIKey: "new-test-key", BaseURL: "https://new.example/v1", Model: "new-chat", EmbeddingModel: "new-embedding",
		}},
		{"clear_key", []string{"--api-key="}, appconfig.LLMConfig{
			BaseURL: "https://old.example/v1", Model: "old-chat", EmbeddingModel: "old-embedding",
		}},
		{"clear_chat", []string{"--model="}, appconfig.LLMConfig{
			APIKey: "old-test-key", BaseURL: "https://old.example/v1", EmbeddingModel: "old-embedding",
		}},
		{"clear_embedding", []string{"--embedding-model="}, appconfig.LLMConfig{
			APIKey: "old-test-key", BaseURL: "https://old.example/v1", Model: "old-chat",
		}},
		{"clear_optional_whitespace", []string{"--api-key", " \t", "--model", " \n", "--embedding-model", " \r\n"}, appconfig.LLMConfig{
			BaseURL: "https://old.example/v1",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(appconfig.EnvAPIKey, "app-environment-only-key")
			t.Setenv(appconfig.OpenAIAPIKeyEnv, "fallback-environment-only-key")
			path := filepath.Join(t.TempDir(), "config.json")
			if err := appconfig.Save(path, configCommandFixture()); err != nil {
				t.Fatal(err)
			}
			output, err := executeConfigCommand(path, append([]string{"set"}, test.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			got, err := appconfig.Load(path)
			if err != nil || got.LLM != test.want {
				t.Fatalf("saved config = %#v, %v; want %#v", got.LLM, err, test.want)
			}
			if output != "saved config: "+path+"\n" {
				t.Fatalf("save output must not echo config values: %q", output)
			}
			if bytes.Contains(readConfigCommandFile(t, path), []byte("environment-only-key")) {
				t.Fatal("an environment key was persisted")
			}
		})
	}
}

func TestConfigSetCreatesConfigWithDefaults(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "environment-only-key")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "fallback-only-key")
	path := filepath.Join(t.TempDir(), "new", "config.json")
	if _, err := executeConfigCommand(path, "set", "--model", "test-chat"); err != nil {
		t.Fatal(err)
	}
	want := appconfig.Default()
	want.LLM.Model = "test-chat"
	got, err := appconfig.Load(path)
	if err != nil || got != want {
		t.Fatalf("new config = %#v, %v; want %#v", got, err, want)
	}
}

func TestConfigInvalidArgumentsDoNotWrite(t *testing.T) {
	for _, args := range [][]string{
		{"set"}, {"set", "--base-url="}, {"set", "--base-url", " \t"},
		{"set", "--api-key", "new-test-secret", "--base-url", "///"},
		{"set", "--model", "new-chat", "extra"}, {"set", "--unknown-flag"},
		{"set", "--model"}, {"path", "extra"}, {"show", "extra"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
					path := filepath.Join(t.TempDir(), "config.json")
					var before []byte
					if existing {
						if err := appconfig.Save(path, configCommandFixture()); err != nil {
							t.Fatal(err)
						}
						before = readConfigCommandFile(t, path)
					}
					output, err := executeConfigCommand(path, args...)
					if err == nil {
						t.Fatal("expected argument error")
					}
					if output != "" || strings.Contains(err.Error(), "new-test-secret") {
						t.Fatalf("invalid input must not report success or echo the key: output=%q, error=%v", output, err)
					}
					if existing {
						if !bytes.Equal(readConfigCommandFile(t, path), before) {
							t.Fatal("invalid arguments modified the config")
						}
					} else if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("invalid arguments created config: %v", err)
					}
				})
			}
		})
	}
}

func TestConfigCorruptFileIsNotOverwritten(t *testing.T) {
	for _, args := range [][]string{{"show"}, {"set", "--model", "new-chat"}} {
		t.Run(args[0], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			before := []byte(`{"llm":{"api_key":"keep-test-secret","model":`)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := executeConfigCommand(path, args...)
			if err == nil || output != "" || strings.Contains(err.Error(), "keep-test-secret") {
				t.Fatalf("corrupt config should fail without printing its key: output=%q, error=%v", output, err)
			}
			if !bytes.Equal(readConfigCommandFile(t, path), before) {
				t.Fatal("corrupt file was overwritten")
			}
		})
	}
}

func TestConfigReportsFilesystemErrors(t *testing.T) {
	t.Run("read_directory", func(t *testing.T) {
		for _, args := range [][]string{{"show"}, {"set", "--model", "test-chat"}} {
			output, err := executeConfigCommand(t.TempDir(), args...)
			if err == nil || output != "" {
				t.Fatalf("directory should fail without success output: %q, %v", output, err)
			}
		}
	})
	t.Run("save_failure", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "parent")
		before := []byte("keep this file")
		if err := os.WriteFile(parent, before, 0o600); err != nil {
			t.Fatal(err)
		}
		output, err := executeConfigCommand(filepath.Join(parent, "config.json"), "set", "--model", "test-chat")
		if err == nil || output != "" {
			t.Fatalf("bad parent should fail without success output: %q, %v", output, err)
		}
		if !bytes.Equal(readConfigCommandFile(t, parent), before) {
			t.Fatal("parent file was changed")
		}
	})
}

func TestConfigCommandsUseSelectedPath(t *testing.T) {
	t.Setenv(appconfig.EnvAPIKey, "")
	t.Setenv(appconfig.OpenAIAPIKeyEnv, "")
	for _, name := range []string{"path", "show", "set"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			defaultPath := filepath.Join(directory, "default.json")
			selectedPath := filepath.Join(directory, "selected config.json")
			cfg := configCommandFixture()
			if err := appconfig.Save(selectedPath, cfg); err != nil {
				t.Fatal(err)
			}
			args := []string{name, "--config", selectedPath}
			if name == "set" {
				args = append(args, "--model", "selected-chat")
				cfg.LLM.Model = "selected-chat"
			}
			output, err := executeConfigCommand(defaultPath, args...)
			if err != nil || !strings.Contains(output, selectedPath) || strings.Contains(output, defaultPath) {
				t.Fatalf("selected config path was ignored: %q, %v", output, err)
			}
			got, err := appconfig.Load(selectedPath)
			if err != nil || got != cfg {
				t.Fatalf("selected config = %#v, %v; want %#v", got, err, cfg)
			}
			if _, err := os.Stat(defaultPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("command touched the default config: %v", err)
			}
		})
	}
}

func executeConfigCommand(path string, args ...string) (string, error) {
	root := &cobra.Command{Use: "adl", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(&path, "config", path, "config path")
	command := &cobra.Command{Use: "config"}
	command.AddCommand(newConfigPathCommand(&path), newConfigShowCommand(&path), newConfigSetCommand(&path))
	root.AddCommand(command)
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs(append([]string{"config"}, args...))
	err := root.Execute()
	return output.String(), err
}

func configCommandFixture() appconfig.Config {
	return appconfig.Config{LLM: appconfig.LLMConfig{
		APIKey: "old-test-key", BaseURL: "https://old.example/v1",
		Model: "old-chat", EmbeddingModel: "old-embedding",
	}}
}

func readConfigCommandFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
