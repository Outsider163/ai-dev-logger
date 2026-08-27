package config

import "testing"

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
