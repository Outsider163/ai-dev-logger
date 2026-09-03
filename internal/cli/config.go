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
	Long:    "管理 API 地址、密钥和模型配置。\n首次配置 DeepSeek 聊天功能建议使用 adl setup，避免在命令历史中留下密钥。",
	Example: "  adl config path\n  adl config show\n  adl config set --help\n  adl setup",
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

			effectiveAPIKey, apiKeySource := appconfig.ResolveAPIKey(cfg.LLM.APIKey)
			apiKey := appconfig.MaskSecret(effectiveAPIKey)
			if reveal {
				apiKey = effectiveAPIKey
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "path: %s\nllm.api_key: %s\nllm.api_key_source: %s\nllm.base_url: %s\nllm.model: %s\nllm.embedding_model: %s\n",
				*path, valueOrEmpty(apiKey), valueOrEmpty(apiKeySource), valueOrEmpty(cfg.LLM.BaseURL), valueOrEmpty(cfg.LLM.Model), valueOrEmpty(cfg.LLM.EmbeddingModel))
			return err
		},
	}
	command.Flags().BoolVar(&reveal, "reveal", false, "显示完整密钥，请勿公开输出")
	return command
}

func newConfigSetCommand(path *string) *cobra.Command {
	var apiKey, baseURL, model, embeddingModel string
	command := &cobra.Command{
		Use:     "set",
		Short:   "修改指定的配置项",
		Long:    "只更新传入的配置项，不修改其他配置。模型名称请替换成服务商实际支持的名称。\n初次设置密钥建议使用 adl setup；本命令只保存配置，不验证接口是否可用。",
		Example: "  adl config set --model \"your-chat-model\"\n  adl config set --embedding-model \"your-embedding-model\"\n  adl doctor --online",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiKeyChanged := cmd.Flags().Changed("api-key")
			baseURLChanged := cmd.Flags().Changed("base-url")
			modelChanged := cmd.Flags().Changed("model")
			embeddingModelChanged := cmd.Flags().Changed("embedding-model")

			if !apiKeyChanged && !baseURLChanged && !modelChanged && !embeddingModelChanged {
				return fmt.Errorf("nothing to set, pass --api-key, --base-url, --model, or --embedding-model")
			}

			cfg, err := appconfig.Load(*path)
			if err != nil {
				return err
			}

			if apiKeyChanged {
				cfg.LLM.APIKey = strings.TrimSpace(apiKey)
			}
			if baseURLChanged {
				cfg.LLM.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
				if cfg.LLM.BaseURL == "" {
					return fmt.Errorf("base-url cannot be empty")
				}
			}
			if modelChanged {
				cfg.LLM.Model = strings.TrimSpace(model)
			}
			if embeddingModelChanged {
				cfg.LLM.EmbeddingModel = strings.TrimSpace(embeddingModel)
			}

			if err := appconfig.Save(*path, cfg); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "saved config: %s\n", *path)
			return err
		},
	}
	command.Flags().StringVar(&apiKey, "api-key", "", "API 密钥，首次配置建议使用 adl setup")
	command.Flags().StringVar(&baseURL, "base-url", "", "API 基础地址")
	command.Flags().StringVar(&model, "model", "", "聊天模型名称，用于 AI 整理和解读")
	command.Flags().StringVar(&embeddingModel, "embedding-model", "", "向量模型名称，用于向量化和语义检索")
	return command
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
