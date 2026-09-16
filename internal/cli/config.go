package cli

import (
	"fmt"
	"strings"

	appconfig "ai-dev-logger/internal/config"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "查看和修改本地配置",
	Long:    "管理聊天和向量服务的独立配置。接口需兼容 OpenAI Chat Completions 与 Embeddings 格式。\n首次配置 DeepSeek 聊天功能建议使用 adl setup，避免在命令历史中留下密钥。",
	Example: "  adl config path\n  adl config show\n  adl config set --chat-model \"your-chat-model\"\n  adl config set --embedding-base-url \"https://example.com/v1\" --embedding-model \"your-embedding-model\"\n  adl setup",
}

var configPathCmd = newConfigPathCommand(&configPath)
var configShowCmd = newConfigShowCommand(&configPath)
var configSetCmd = newConfigSetCommand(&configPath)

func newConfigPathCommand(path *string) *cobra.Command {
	return &cobra.Command{
		Use:     "path",
		Short:   "显示配置文件路径",
		Example: "  adl config path",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), *path)
			return err
		},
	}
}

func newConfigShowCommand(path *string) *cobra.Command {
	var reveal bool
	command := &cobra.Command{
		Use:     "show",
		Short:   "查看配置和密钥来源",
		Long:    "显示生效配置，默认对 API Key 脱敏。\n--reveal 会显示完整密钥，请勿将输出截图或日志公开。",
		Example: "  adl config show",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := appconfig.Load(*path)
			if err != nil {
				return err
			}
			if !cfg.HasProviderProfiles() {
				effectiveAPIKey, apiKeySource := appconfig.ResolveAPIKey(cfg.LLM.APIKey)
				if !reveal {
					effectiveAPIKey = appconfig.MaskSecret(effectiveAPIKey)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "path: %s\nllm.api_key: %s\nllm.api_key_source: %s\nllm.base_url: %s\nllm.model: %s\nllm.embedding_model: %s\n",
					*path, valueOrEmpty(effectiveAPIKey), valueOrEmpty(apiKeySource), valueOrEmpty(cfg.LLM.BaseURL), valueOrEmpty(cfg.LLM.Model), valueOrEmpty(cfg.LLM.EmbeddingModel))
				return err
			}

			chat := cfg.ChatProvider()
			embedding := cfg.EmbeddingProvider()
			chatKey, chatKeySource := appconfig.ResolveChatAPIKey(chat.APIKey)
			embeddingKey, embeddingKeySource := appconfig.ResolveEmbeddingAPIKey(embedding.APIKey)
			if !reveal {
				chatKey = appconfig.MaskSecret(chatKey)
				embeddingKey = appconfig.MaskSecret(embeddingKey)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "path: %s\nchat.api_key: %s\nchat.api_key_source: %s\nchat.base_url: %s\nchat.model: %s\nembedding.api_key: %s\nembedding.api_key_source: %s\nembedding.base_url: %s\nembedding.model: %s\n",
				*path,
				valueOrEmpty(chatKey), valueOrEmpty(chatKeySource), valueOrEmpty(chat.BaseURL), valueOrEmpty(chat.Model),
				valueOrEmpty(embeddingKey), valueOrEmpty(embeddingKeySource), valueOrEmpty(embedding.BaseURL), valueOrEmpty(embedding.Model))
			return err
		},
	}
	command.Flags().BoolVar(&reveal, "reveal", false, "显示完整密钥，请勿公开输出")
	return command
}

func newConfigSetCommand(path *string) *cobra.Command {
	var apiKey, baseURL, model string
	var chatAPIKey, chatBaseURL, chatModel string
	var embeddingAPIKey, embeddingBaseURL, embeddingModel string
	command := &cobra.Command{
		Use:     "set",
		Short:   "修改指定的配置项",
		Long:    "只更新传入的配置项，不修改其他配置。聊天和向量服务可使用不同供应商。\n初次设置聊天密钥建议使用 adl setup；本命令只保存配置，不验证接口是否可用。",
		Example: "  adl config set --chat-base-url \"https://api.example.com/v1\" --chat-model \"chat-model\"\n  adl config set --embedding-base-url \"https://embed.example.com/v1\" --embedding-model \"embedding-model\"\n  adl doctor --online",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sharedAPIKeyChanged := cmd.Flags().Changed("api-key")
			sharedBaseURLChanged := cmd.Flags().Changed("base-url")
			modelChanged := cmd.Flags().Changed("model")
			chatAPIKeyChanged := cmd.Flags().Changed("chat-api-key")
			chatBaseURLChanged := cmd.Flags().Changed("chat-base-url")
			chatModelChanged := cmd.Flags().Changed("chat-model")
			embeddingAPIKeyChanged := cmd.Flags().Changed("embedding-api-key")
			embeddingBaseURLChanged := cmd.Flags().Changed("embedding-base-url")
			embeddingModelChanged := cmd.Flags().Changed("embedding-model")
			profileFlagsChanged := chatAPIKeyChanged || chatBaseURLChanged || chatModelChanged || embeddingAPIKeyChanged || embeddingBaseURLChanged

			if !sharedAPIKeyChanged && !sharedBaseURLChanged && !modelChanged && !chatAPIKeyChanged && !chatBaseURLChanged && !chatModelChanged && !embeddingAPIKeyChanged && !embeddingBaseURLChanged && !embeddingModelChanged {
				return fmt.Errorf("nothing to set, pass a chat or embedding configuration flag")
			}

			cfg, err := appconfig.Load(*path)
			if err != nil {
				return err
			}

			if !profileFlagsChanged && !cfg.HasProviderProfiles() {
				if sharedAPIKeyChanged {
					cfg.LLM.APIKey = strings.TrimSpace(apiKey)
				}
				if sharedBaseURLChanged {
					url, err := normalizedProviderURL(baseURL, "base-url")
					if err != nil {
						return err
					}
					cfg.LLM.BaseURL = url
				}
				if modelChanged {
					cfg.LLM.Model = strings.TrimSpace(model)
				}
				if embeddingModelChanged {
					cfg.LLM.EmbeddingModel = strings.TrimSpace(embeddingModel)
				}
			} else {
				cfg.MaterializeProviderProfiles()
				if sharedAPIKeyChanged {
					key := strings.TrimSpace(apiKey)
					cfg.Chat.APIKey, cfg.Embedding.APIKey = key, key
				}
				if sharedBaseURLChanged {
					url, err := normalizedProviderURL(baseURL, "base-url")
					if err != nil {
						return err
					}
					cfg.Chat.BaseURL, cfg.Embedding.BaseURL = url, url
				}
				if modelChanged {
					cfg.Chat.Model = strings.TrimSpace(model)
				}
				if chatAPIKeyChanged {
					cfg.Chat.APIKey = strings.TrimSpace(chatAPIKey)
				}
				if chatBaseURLChanged {
					url, err := normalizedProviderURL(chatBaseURL, "chat-base-url")
					if err != nil {
						return err
					}
					cfg.Chat.BaseURL = url
				}
				if chatModelChanged {
					cfg.Chat.Model = strings.TrimSpace(chatModel)
				}
				if embeddingAPIKeyChanged {
					cfg.Embedding.APIKey = strings.TrimSpace(embeddingAPIKey)
				}
				if embeddingBaseURLChanged {
					url, err := normalizedProviderURL(embeddingBaseURL, "embedding-base-url")
					if err != nil {
						return err
					}
					cfg.Embedding.BaseURL = url
				}
				if embeddingModelChanged {
					cfg.Embedding.Model = strings.TrimSpace(embeddingModel)
				}
			}

			if err := appconfig.Save(*path, cfg); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "saved config: %s\n", *path)
			return err
		},
	}
	command.Flags().StringVar(&chatAPIKey, "chat-api-key", "", "聊天服务 API 密钥")
	command.Flags().StringVar(&chatBaseURL, "chat-base-url", "", "聊天服务 API 基础地址")
	command.Flags().StringVar(&chatModel, "chat-model", "", "聊天模型名称，用于 AI 整理和回答")
	command.Flags().StringVar(&embeddingAPIKey, "embedding-api-key", "", "向量服务 API 密钥")
	command.Flags().StringVar(&embeddingBaseURL, "embedding-base-url", "", "向量服务 API 基础地址")
	command.Flags().StringVar(&embeddingModel, "embedding-model", "", "向量模型名称，用于向量化和语义检索")
	command.Flags().StringVar(&apiKey, "api-key", "", "兼容旧配置：同时设置聊天和向量 API 密钥")
	command.Flags().StringVar(&baseURL, "base-url", "", "兼容旧配置：同时设置聊天和向量 API 基础地址")
	command.Flags().StringVar(&model, "model", "", "兼容旧配置：设置聊天模型")
	return command
}

func normalizedProviderURL(value, flag string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", flag)
	}
	return value, nil
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
}

func valueOrEmpty(value string) string {
	if value == "" {
		return "(empty)"
	}
	return value
}
