package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvAPIKey            = "AI_DEV_LOGGER_API_KEY"
	EnvChatAPIKey        = "AI_DEV_LOGGER_CHAT_API_KEY"
	EnvEmbeddingAPIKey   = "AI_DEV_LOGGER_EMBEDDING_API_KEY"
	OpenAIAPIKeyEnv      = "OPENAI_API_KEY"
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
)

type Config struct {
	// LLM is retained to read configurations written before provider profiles.
	// New configurations use Chat and Embedding independently.
	LLM       LLMConfig      `json:"llm"`
	Chat      ProviderConfig `json:"chat"`
	Embedding ProviderConfig `json:"embedding"`
}

type LLMConfig struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

// ProviderConfig is one OpenAI-compatible endpoint and its selected model.
// The provider is identified by its base URL, so custom compatible gateways work too.
type ProviderConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			BaseURL: defaultOpenAIBaseURL,
		},
	}
}

func DefaultPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "ai-dev-logger.json"
	}

	return filepath.Join(configDir, "ai-dev-logger", "config.json")
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if len(data) == 0 {
		return cfg, nil
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = Default().LLM.BaseURL
	}

	return cfg, nil
}

// ChatProvider returns the effective chat profile, falling back to legacy llm values.
func (cfg Config) ChatProvider() ProviderConfig {
	return mergeProvider(cfg.Chat, ProviderConfig{
		APIKey: cfg.LLM.APIKey, BaseURL: cfg.LLM.BaseURL, Model: cfg.LLM.Model,
	})
}

// EmbeddingProvider returns the effective embedding profile, falling back to legacy llm values.
func (cfg Config) EmbeddingProvider() ProviderConfig {
	return mergeProvider(cfg.Embedding, ProviderConfig{
		APIKey: cfg.LLM.APIKey, BaseURL: cfg.LLM.BaseURL, Model: cfg.LLM.EmbeddingModel,
	})
}

// ChatRuntimeProvider preserves legacy environment precedence without persisting it.
func (cfg Config) ChatRuntimeProvider() ProviderConfig {
	provider := cfg.ChatProvider()
	if !cfg.HasProviderProfiles() {
		provider.APIKey, _ = ResolveAPIKey(cfg.LLM.APIKey)
	}
	return provider
}

// EmbeddingRuntimeProvider preserves legacy environment precedence without persisting it.
func (cfg Config) EmbeddingRuntimeProvider() ProviderConfig {
	provider := cfg.EmbeddingProvider()
	if !cfg.HasProviderProfiles() {
		provider.APIKey, _ = ResolveAPIKey(cfg.LLM.APIKey)
	}
	return provider
}

func mergeProvider(profile ProviderConfig, legacy ProviderConfig) ProviderConfig {
	if strings.TrimSpace(profile.APIKey) == "" {
		profile.APIKey = legacy.APIKey
	}
	if strings.TrimSpace(profile.BaseURL) == "" {
		profile.BaseURL = legacy.BaseURL
	}
	if strings.TrimSpace(profile.Model) == "" {
		profile.Model = legacy.Model
	}
	if strings.TrimSpace(profile.BaseURL) == "" {
		profile.BaseURL = defaultOpenAIBaseURL
	}
	profile.APIKey = strings.TrimSpace(profile.APIKey)
	profile.BaseURL = strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	profile.Model = strings.TrimSpace(profile.Model)
	return profile
}

// MaterializeProviderProfiles upgrades legacy shared settings before a profile edit.
func (cfg *Config) MaterializeProviderProfiles() {
	cfg.Chat = cfg.ChatProvider()
	cfg.Embedding = cfg.EmbeddingProvider()
	cfg.LLM = LLMConfig{}
}

// HasProviderProfiles reports whether this configuration uses separate service profiles.
func (cfg Config) HasProviderProfiles() bool {
	return cfg.Chat != (ProviderConfig{}) || cfg.Embedding != (ProviderConfig{})
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o600)
}

func MaskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}

	return value[:4] + "..." + value[len(value)-4:]
}

// ResolveAPIKey returns the effective API key and a human-readable source.
func ResolveAPIKey(configured string) (string, string) {
	if value := strings.TrimSpace(os.Getenv(EnvAPIKey)); value != "" {
		return value, EnvAPIKey
	}
	if value := strings.TrimSpace(configured); value != "" {
		return value, "config file"
	}
	if value := strings.TrimSpace(os.Getenv(OpenAIAPIKeyEnv)); value != "" {
		return value, OpenAIAPIKeyEnv
	}
	return "", ""
}

// ResolveChatAPIKey prefers chat-specific settings before legacy shared fallbacks.
func ResolveChatAPIKey(configured string) (string, string) {
	return resolveProviderAPIKey(EnvChatAPIKey, configured)
}

// ResolveEmbeddingAPIKey prefers embedding-specific settings before legacy shared fallbacks.
func ResolveEmbeddingAPIKey(configured string) (string, string) {
	return resolveProviderAPIKey(EnvEmbeddingAPIKey, configured)
}

func resolveProviderAPIKey(specificEnvironment string, configured string) (string, string) {
	if value := strings.TrimSpace(os.Getenv(specificEnvironment)); value != "" {
		return value, specificEnvironment
	}
	if value := strings.TrimSpace(configured); value != "" {
		return value, "config file"
	}
	if value := strings.TrimSpace(os.Getenv(EnvAPIKey)); value != "" {
		return value, EnvAPIKey
	}
	if value := strings.TrimSpace(os.Getenv(OpenAIAPIKeyEnv)); value != "" {
		return value, OpenAIAPIKeyEnv
	}
	return "", ""
}
