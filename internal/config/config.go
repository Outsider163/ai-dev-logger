package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvAPIKey       = "AI_DEV_LOGGER_API_KEY"
	OpenAIAPIKeyEnv = "OPENAI_API_KEY"
)

type Config struct {
	LLM LLMConfig `json:"llm"`
}

type LLMConfig struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

func Default() Config {
	return Config{
		LLM: LLMConfig{
			BaseURL: "https://api.openai.com/v1",
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
