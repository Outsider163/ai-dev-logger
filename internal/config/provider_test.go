package config

import "testing"

func TestProviderProfilesFallBackToLegacyAndOverrideIndependently(t *testing.T) {
	cfg := Config{LLM: LLMConfig{
		APIKey: "legacy-key", BaseURL: "https://legacy.example/v1",
		Model: "legacy-chat", EmbeddingModel: "legacy-embedding",
	}}
	if got := cfg.ChatProvider(); got != (ProviderConfig{APIKey: "legacy-key", BaseURL: "https://legacy.example/v1", Model: "legacy-chat"}) {
		t.Fatalf("legacy chat profile = %#v", got)
	}
	if got := cfg.EmbeddingProvider(); got != (ProviderConfig{APIKey: "legacy-key", BaseURL: "https://legacy.example/v1", Model: "legacy-embedding"}) {
		t.Fatalf("legacy embedding profile = %#v", got)
	}
	cfg.Embedding = ProviderConfig{APIKey: "embedding-key", BaseURL: "https://embed.example/v1", Model: "embed-model"}
	if got := cfg.ChatProvider(); got.Model != "legacy-chat" || got.APIKey != "legacy-key" {
		t.Fatalf("embedding override changed chat profile: %#v", got)
	}
	if got := cfg.EmbeddingProvider(); got != cfg.Embedding {
		t.Fatalf("embedding override = %#v, want %#v", got, cfg.Embedding)
	}
}

func TestMaterializeProviderProfilesCopiesLegacyOnce(t *testing.T) {
	cfg := Config{LLM: LLMConfig{APIKey: "key", BaseURL: "https://example.test/v1", Model: "chat", EmbeddingModel: "embed"}}
	cfg.MaterializeProviderProfiles()
	if cfg.LLM != (LLMConfig{}) || !cfg.HasProviderProfiles() {
		t.Fatalf("legacy data was not migrated: %#v", cfg)
	}
	if cfg.Chat != (ProviderConfig{APIKey: "key", BaseURL: "https://example.test/v1", Model: "chat"}) {
		t.Fatalf("chat profile = %#v", cfg.Chat)
	}
	if cfg.Embedding != (ProviderConfig{APIKey: "key", BaseURL: "https://example.test/v1", Model: "embed"}) {
		t.Fatalf("embedding profile = %#v", cfg.Embedding)
	}
}

func TestResolveProviderAPIKeysPreferSpecificEnvironment(t *testing.T) {
	t.Setenv(EnvAPIKey, "shared-key")
	t.Setenv(OpenAIAPIKeyEnv, "openai-key")
	t.Setenv(EnvChatAPIKey, "chat-key")
	t.Setenv(EnvEmbeddingAPIKey, "embedding-key")
	if key, source := ResolveChatAPIKey("configured-chat"); key != "chat-key" || source != EnvChatAPIKey {
		t.Fatalf("chat key = %q from %q", key, source)
	}
	if key, source := ResolveEmbeddingAPIKey("configured-embedding"); key != "embedding-key" || source != EnvEmbeddingAPIKey {
		t.Fatalf("embedding key = %q from %q", key, source)
	}
	t.Setenv(EnvChatAPIKey, "")
	t.Setenv(EnvEmbeddingAPIKey, "")
	if key, source := ResolveChatAPIKey("configured-chat"); key != "configured-chat" || source != "config file" {
		t.Fatalf("chat configured key = %q from %q", key, source)
	}
	if key, source := ResolveEmbeddingAPIKey("configured-embedding"); key != "configured-embedding" || source != "config file" {
		t.Fatalf("embedding configured key = %q from %q", key, source)
	}
	if key, source := ResolveChatAPIKey(" "); key != "shared-key" || source != EnvAPIKey {
		t.Fatalf("chat shared fallback = %q from %q", key, source)
	}
	if key, source := ResolveEmbeddingAPIKey(""); key != "shared-key" || source != EnvAPIKey {
		t.Fatalf("embedding shared fallback = %q from %q", key, source)
	}
}

func TestRuntimeProvidersKeepLegacyAndProfilePrecedenceSeparate(t *testing.T) {
	t.Setenv(EnvAPIKey, "legacy-environment-key")
	t.Setenv(EnvChatAPIKey, "")
	t.Setenv(EnvEmbeddingAPIKey, "")
	t.Setenv(OpenAIAPIKeyEnv, "")

	legacy := Config{LLM: LLMConfig{
		APIKey: "legacy-file-key", BaseURL: "https://legacy.example/v1",
		Model: "legacy-chat", EmbeddingModel: "legacy-embedding",
	}}
	if got := legacy.ChatRuntimeProvider().APIKey; got != "legacy-environment-key" {
		t.Fatalf("legacy chat key = %q", got)
	}
	if got := legacy.EmbeddingRuntimeProvider().APIKey; got != "legacy-environment-key" {
		t.Fatalf("legacy embedding key = %q", got)
	}

	profiles := Config{
		Chat:      ProviderConfig{APIKey: "chat-file-key", BaseURL: "https://chat.example/v1", Model: "chat"},
		Embedding: ProviderConfig{APIKey: "embedding-file-key", BaseURL: "https://embedding.example/v1", Model: "embedding"},
	}
	if got := profiles.ChatRuntimeProvider().APIKey; got != "chat-file-key" {
		t.Fatalf("profile chat key = %q", got)
	}
	if got := profiles.EmbeddingRuntimeProvider().APIKey; got != "embedding-file-key" {
		t.Fatalf("profile embedding key = %q", got)
	}

	profiles.MaterializeProviderProfiles()
	if profiles.Chat.APIKey != "chat-file-key" || profiles.Embedding.APIKey != "embedding-file-key" {
		t.Fatalf("materializing profiles persisted an environment key: %#v", profiles)
	}
}
