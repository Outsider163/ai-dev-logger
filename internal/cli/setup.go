package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	deepSeekBaseURL      = "https://api.deepseek.com"
	deepSeekDefaultModel = "deepseek-v4-flash"
	defaultSetupTimeout  = 30 * time.Second
)

var setupSkipTest bool
var setupTimeout time.Duration

type setupOptions struct {
	ConfigPath string
	Input      io.Reader
	Output     io.Writer
	SkipTest   bool
	Timeout    time.Duration
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure DeepSeek chat access interactively",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup(cmd.Context(), setupOptions{
			ConfigPath: configPath,
			Input:      cmd.InOrStdin(),
			Output:     cmd.OutOrStdout(),
			SkipTest:   setupSkipTest,
			Timeout:    setupTimeout,
		})
	},
}

func init() {
	setupCmd.Flags().BoolVar(&setupSkipTest, "skip-test", false, "Save without testing the DeepSeek chat API")
	setupCmd.Flags().DurationVar(&setupTimeout, "timeout", defaultSetupTimeout, "Connection test timeout")
}

func runSetup(ctx context.Context, options setupOptions) error {
	if options.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}

	cfg, err := appconfig.Load(options.ConfigPath)
	if err != nil {
		return err
	}

	fmt.Fprintln(options.Output, "AI Dev Logger DeepSeek setup")
	fmt.Fprintln(options.Output, "A newly entered API key will be stored in your local config file.")

	reader := bufio.NewReader(options.Input)
	effectiveKey, _ := appconfig.ResolveAPIKey(cfg.LLM.APIKey)
	apiKey, err := readSetupSecret(options.Input, reader, options.Output, effectiveKey != "")
	if err != nil {
		return err
	}
	if apiKey != "" {
		cfg.LLM.APIKey = apiKey
	}

	effectiveKey, keySource := appconfig.ResolveAPIKey(cfg.LLM.APIKey)
	if effectiveKey == "" {
		return fmt.Errorf("DeepSeek API key is required")
	}

	baseURLDefault := deepSeekBaseURL
	currentBaseURL := strings.TrimRight(strings.TrimSpace(cfg.LLM.BaseURL), "/")
	if currentBaseURL != "" && currentBaseURL != appconfig.Default().LLM.BaseURL {
		baseURLDefault = currentBaseURL
	}
	baseURL, err := readSetupPrompt(reader, options.Output, "DeepSeek base URL", baseURLDefault)
	if err != nil {
		return err
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if err := validateHTTPBaseURL(baseURL); err != nil {
		return err
	}

	modelDefault := deepSeekDefaultModel
	if currentModel := strings.TrimSpace(cfg.LLM.Model); strings.HasPrefix(currentModel, "deepseek-") {
		modelDefault = currentModel
	}
	model, err := readSetupPrompt(reader, options.Output, "DeepSeek chat model", modelDefault)
	if err != nil {
		return err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("DeepSeek chat model is required")
	}

	cfg.LLM.BaseURL = baseURL
	cfg.LLM.Model = model
	if isOfficialDeepSeekBaseURL(baseURL) {
		cfg.LLM.EmbeddingModel = ""
	}

	if options.SkipTest {
		fmt.Fprintln(options.Output, "connection test: skipped")
	} else {
		fmt.Fprintln(options.Output, "Testing DeepSeek chat connection...")
		probeContext, cancel := context.WithTimeout(ctx, options.Timeout)
		err := llm.NewClient(cfg.LLM).ProbeChat(probeContext)
		cancel()
		if err != nil {
			return fmt.Errorf("DeepSeek chat test failed; configuration was not saved: %w", err)
		}
		fmt.Fprintln(options.Output, "connection test: passed")
	}

	if err := appconfig.Save(options.ConfigPath, cfg); err != nil {
		return err
	}

	fmt.Fprintf(options.Output, "saved config: %s\n", options.ConfigPath)
	fmt.Fprintf(options.Output, "API key source: %s\n", keySource)
	fmt.Fprintf(options.Output, "chat model: %s\n", cfg.LLM.Model)
	if isOfficialDeepSeekBaseURL(baseURL) {
		fmt.Fprintln(options.Output, "embedding model: not configured (DeepSeek chat setup only)")
	}
	fmt.Fprintln(options.Output, "Try: adl add --ai --title \"test\" --body \"hello\"")
	return nil
}

func readSetupSecret(
	input io.Reader,
	reader *bufio.Reader,
	output io.Writer,
	hasExistingKey bool,
) (string, error) {
	prompt := "DeepSeek API key"
	if hasExistingKey {
		prompt += " (press Enter to keep the current key)"
	}
	fmt.Fprintf(output, "%s: ", prompt)

	if descriptor, ok := input.(interface{ Fd() uintptr }); ok {
		fd := int(descriptor.Fd())
		if term.IsTerminal(fd) {
			value, err := term.ReadPassword(fd)
			fmt.Fprintln(output)
			if err != nil {
				return "", fmt.Errorf("read API key: %w", err)
			}
			return strings.TrimSpace(string(value)), nil
		}
	}

	value, err := readSetupLine(reader)
	if err != nil {
		return "", fmt.Errorf("read API key: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func readSetupPrompt(reader *bufio.Reader, output io.Writer, label string, defaultValue string) (string, error) {
	fmt.Fprintf(output, "%s [%s]: ", label, defaultValue)
	value, err := readSetupLine(reader)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func readSetupLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func isOfficialDeepSeekBaseURL(value string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "/"))
	return normalized == deepSeekBaseURL || normalized == deepSeekBaseURL+"/v1"
}
