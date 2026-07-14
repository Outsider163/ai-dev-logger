package cli

import (
	"fmt"
	"strings"

	appconfig "ai-dev-logger/internal/config"

	"github.com/spf13/cobra"
)

var configSetAPIKey string
var configSetBaseURL string
var configSetModel string
var configSetEmbeddingModel string
var configShowReveal bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage local configuration",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(configPath)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show local configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}

		apiKey := appconfig.MaskSecret(cfg.LLM.APIKey)
		if configShowReveal {
			apiKey = cfg.LLM.APIKey
		}
		if apiKey == "" {
			apiKey = "(empty)"
		}

		fmt.Printf("path: %s\n", configPath)
		fmt.Printf("llm.api_key: %s\n", apiKey)
		fmt.Printf("llm.base_url: %s\n", valueOrEmpty(cfg.LLM.BaseURL))
		fmt.Printf("llm.model: %s\n", valueOrEmpty(cfg.LLM.Model))
		fmt.Printf("llm.embedding_model: %s\n", valueOrEmpty(cfg.LLM.EmbeddingModel))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Update local configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKeyChanged := cmd.Flags().Changed("api-key")
		baseURLChanged := cmd.Flags().Changed("base-url")
		modelChanged := cmd.Flags().Changed("model")
		embeddingModelChanged := cmd.Flags().Changed("embedding-model")

		if !apiKeyChanged && !baseURLChanged && !modelChanged && !embeddingModelChanged {
			return fmt.Errorf("nothing to set, pass --api-key, --base-url, --model, or --embedding-model")
		}

		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}

		if apiKeyChanged {
			cfg.LLM.APIKey = strings.TrimSpace(configSetAPIKey)
		}
		if baseURLChanged {
			cfg.LLM.BaseURL = strings.TrimRight(strings.TrimSpace(configSetBaseURL), "/")
			if cfg.LLM.BaseURL == "" {
				return fmt.Errorf("base-url cannot be empty")
			}
		}
		if modelChanged {
			cfg.LLM.Model = strings.TrimSpace(configSetModel)
		}
		if embeddingModelChanged {
			cfg.LLM.EmbeddingModel = strings.TrimSpace(configSetEmbeddingModel)
		}

		if err := appconfig.Save(configPath, cfg); err != nil {
			return err
		}

		fmt.Printf("saved config: %s\n", configPath)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)

	configShowCmd.Flags().BoolVar(&configShowReveal, "reveal", false, "Print secrets in plain text")

	configSetCmd.Flags().StringVar(&configSetAPIKey, "api-key", "", "LLM API key")
	configSetCmd.Flags().StringVar(&configSetBaseURL, "base-url", "", "LLM API base URL")
	configSetCmd.Flags().StringVar(&configSetModel, "model", "", "LLM chat model")
	configSetCmd.Flags().StringVar(&configSetEmbeddingModel, "embedding-model", "", "LLM embedding model")
}

func valueOrEmpty(value string) string {
	if value == "" {
		return "(empty)"
	}
	return value
}
