package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

const defaultDoctorTimeout = 15 * time.Second

var doctorOnline bool
var doctorTimeout = defaultDoctorTimeout

type doctorStatus string

const (
	doctorPass doctorStatus = "PASS"
	doctorWarn doctorStatus = "WARN"
	doctorFail doctorStatus = "FAIL"
	doctorSkip doctorStatus = "SKIP"
)

type doctorCheck struct {
	Status doctorStatus
	Name   string
	Detail string
}

type doctorOptions struct {
	ConfigPath string
	DBPath     string
	Online     bool
	Timeout    time.Duration
}

type doctorReport struct {
	checks       []doctorCheck
	capabilities []doctorCheck
}

type doctorReadiness struct {
	databaseReady bool
	configReady   bool
	legacyConfig  bool
	chat          providerReadiness
	embedding     providerReadiness
}

type providerReadiness struct {
	apiKeyReady  bool
	baseURLReady bool
	modelReady   bool
	apiStatus    doctorStatus
}

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Short:   "检查本地存储和可选 AI 能力",
	Long:    "检查数据库、配置文件，以及聊天和向量功能的配置。\n未配置可选 AI 功能只会提示 WARN；默认不联网，--online 仅验证已配置完整的接口。",
	Example: "  adl doctor\n  adl doctor --online --timeout 20s",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(cmd.Context(), cmd.OutOrStdout(), doctorOptions{
			ConfigPath: configPath,
			DBPath:     dbPath,
			Online:     doctorOnline,
			Timeout:    doctorTimeout,
		})
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorOnline, "online", false, "验证已配置的聊天和向量接口，可能产生 API 用量")
	doctorCmd.Flags().DurationVar(&doctorTimeout, "timeout", defaultDoctorTimeout, "每个在线检查的最长等待时间，包含重试")
}

func runDoctor(ctx context.Context, writer io.Writer, options doctorOptions) error {
	if options.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than zero")
	}

	report := doctorReport{}
	cfg, configReady, configCheck := inspectConfig(options.ConfigPath)
	report.add(configCheck)
	databaseCheck := inspectDatabase(options.DBPath)
	report.add(databaseCheck)
	readiness := doctorReadiness{
		databaseReady: databaseCheck.Status == doctorPass,
		configReady:   configReady,
		legacyConfig:  true,
	}

	if !configReady {
		report.addSkippedLegacyConfigChecks()
	} else {
		if cfg.HasProviderProfiles() {
			readiness.legacyConfig = false
			readiness.chat = inspectProvider(&report, "chat", cfg.ChatProvider(), appconfig.ResolveChatAPIKey)
			readiness.embedding = inspectProvider(&report, "embedding", cfg.EmbeddingProvider(), appconfig.ResolveEmbeddingAPIKey)
		} else {
			readiness.chat, readiness.embedding = inspectLegacyLLM(&report, cfg.LLM)
		}
	}

	if !options.Online {
		report.add(doctorCheck{
			Status: doctorSkip,
			Name:   "online probes",
			Detail: "not requested; run adl doctor --online",
		})
	} else if !configReady {
		report.add(doctorCheck{doctorSkip, "chat API", "configuration could not be loaded"})
		report.add(doctorCheck{doctorSkip, "embedding API", "configuration could not be loaded"})
	} else {
		if readiness.chat.ready() {
			check := probeChat(ctx, llm.NewChatClient(cfg.ChatRuntimeProvider()), options.Timeout)
			readiness.chat.apiStatus = check.Status
			report.add(check)
		} else {
			report.add(doctorCheck{doctorSkip, "chat API", "required settings are not ready"})
		}
		if readiness.embedding.ready() {
			check := probeEmbedding(ctx, llm.NewEmbeddingClient(cfg.EmbeddingRuntimeProvider()), options.Timeout)
			readiness.embedding.apiStatus = check.Status
			report.add(check)
		} else {
			report.add(doctorCheck{doctorSkip, "embedding API", "required settings are not ready"})
		}
	}

	report.capabilities = readiness.capabilities()
	failed := report.write(writer)
	if failed > 0 {
		return fmt.Errorf("doctor found %d failed check(s)", failed)
	}
	return nil
}

func inspectLegacyLLM(report *doctorReport, cfg appconfig.LLMConfig) (providerReadiness, providerReadiness) {
	ready := providerReadiness{}
	apiKey, source := appconfig.ResolveAPIKey(cfg.APIKey)
	ready.apiKeyReady = apiKey != ""
	if ready.apiKeyReady {
		report.add(doctorCheck{doctorPass, "API key", "configured via " + source})
	} else {
		report.add(doctorCheck{doctorWarn, "API key", "not configured (optional); set " + appconfig.EnvAPIKey + " or run adl config set --api-key"})
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if err := validateHTTPBaseURL(baseURL); err != nil {
		report.add(doctorCheck{doctorFail, "LLM base URL", err.Error()})
	} else {
		ready.baseURLReady = true
		report.add(doctorCheck{doctorPass, "LLM base URL", baseURL})
	}
	chatReady := ready
	chatReady.modelReady = strings.TrimSpace(cfg.Model) != ""
	if chatReady.modelReady {
		report.add(doctorCheck{doctorPass, "chat model", strings.TrimSpace(cfg.Model)})
	} else {
		report.add(doctorCheck{doctorWarn, "chat model", "not configured (optional); run adl setup or adl config set --model"})
	}
	embeddingReady := ready
	embeddingReady.modelReady = strings.TrimSpace(cfg.EmbeddingModel) != ""
	if embeddingReady.modelReady {
		report.add(doctorCheck{doctorPass, "embedding model", strings.TrimSpace(cfg.EmbeddingModel)})
	} else {
		report.add(doctorCheck{doctorWarn, "embedding model", "not configured (optional); needed for semantic search; run adl config set --embedding-model"})
	}
	return chatReady, embeddingReady
}

func inspectProvider(report *doctorReport, name string, provider appconfig.ProviderConfig, resolveKey func(string) (string, string)) providerReadiness {
	ready := providerReadiness{}
	apiKey, source := resolveKey(provider.APIKey)
	ready.apiKeyReady = apiKey != ""
	if ready.apiKeyReady {
		report.add(doctorCheck{doctorPass, name + " API key", "configured via " + source})
	} else {
		report.add(doctorCheck{doctorWarn, name + " API key", "not configured (optional); set a specific environment variable or use adl config set"})
	}
	baseURL := strings.TrimSpace(provider.BaseURL)
	if err := validateHTTPBaseURL(baseURL); err != nil {
		report.add(doctorCheck{doctorFail, name + " base URL", err.Error()})
	} else {
		ready.baseURLReady = true
		report.add(doctorCheck{doctorPass, name + " base URL", baseURL})
	}
	model := strings.TrimSpace(provider.Model)
	ready.modelReady = model != ""
	if ready.modelReady {
		report.add(doctorCheck{doctorPass, name + " model", model})
	} else {
		report.add(doctorCheck{doctorWarn, name + " model", "not configured (optional); run adl config set"})
	}
	return ready
}

func (r providerReadiness) ready() bool {
	return r.apiKeyReady && r.baseURLReady && r.modelReady
}

func inspectConfig(path string) (appconfig.Config, bool, doctorCheck) {
	_, statErr := os.Stat(path)
	cfg, loadErr := appconfig.Load(path)
	if loadErr != nil {
		return appconfig.Config{}, false, doctorCheck{doctorFail, "config", fmt.Sprintf("%s: %v", path, loadErr)}
	}
	if errors.Is(statErr, os.ErrNotExist) {
		return cfg, true, doctorCheck{doctorWarn, "config", "not found; defaults loaded from " + path}
	}
	if statErr != nil {
		return appconfig.Config{}, false, doctorCheck{doctorFail, "config", fmt.Sprintf("%s: %v", path, statErr)}
	}
	return cfg, true, doctorCheck{doctorPass, "config", path}
}

func inspectDatabase(path string) doctorCheck {
	_, statErr := os.Stat(path)
	wasMissing := errors.Is(statErr, os.ErrNotExist)

	db, err := store.Open(path)
	if err != nil {
		return doctorCheck{doctorFail, "database", fmt.Sprintf("%s: %v", path, err)}
	}
	if err := db.Close(); err != nil {
		return doctorCheck{doctorFail, "database", fmt.Sprintf("close %s: %v", path, err)}
	}
	if wasMissing {
		return doctorCheck{doctorPass, "database", "initialized: " + path}
	}
	return doctorCheck{doctorPass, "database", "ready: " + path}
}

func validateHTTPBaseURL(value string) error {
	if value == "" {
		return fmt.Errorf("missing; run adl config set --base-url")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid HTTP(S) URL %q; run adl config set --base-url", value)
	}
	return nil
}

func probeChat(ctx context.Context, client *llm.Client, timeout time.Duration) doctorCheck {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.ProbeChat(probeCtx); err != nil {
		return doctorCheck{doctorFail, "chat API", err.Error()}
	}
	return doctorCheck{doctorPass, "chat API", "request succeeded"}
}

func probeEmbedding(ctx context.Context, client *llm.Client, timeout time.Duration) doctorCheck {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dimensions, err := client.ProbeEmbedding(probeCtx)
	if err != nil {
		return doctorCheck{doctorFail, "embedding API", err.Error()}
	}
	return doctorCheck{doctorPass, "embedding API", fmt.Sprintf("request succeeded (%d dimensions)", dimensions)}
}

func (r *doctorReport) add(check doctorCheck) {
	r.checks = append(r.checks, check)
}

func (r *doctorReport) addSkippedLegacyConfigChecks() {
	for _, name := range []string{"API key", "LLM base URL", "chat model", "embedding model"} {
		r.add(doctorCheck{doctorSkip, name, "configuration could not be loaded"})
	}
}

func (r doctorReadiness) capabilities() []doctorCheck {
	local := doctorCheck{doctorPass, "local notes", "required; ready (notes and keyword search)"}
	if !r.databaseReady {
		local = doctorCheck{doctorFail, "local notes", "required; unavailable; database is not ready"}
	}

	return []doctorCheck{
		local,
		r.optionalCapability("AI enhancement", r.chat),
		r.optionalCapability("semantic search", r.embedding),
	}
}

func (r doctorReadiness) optionalCapability(name string, provider providerReadiness) doctorCheck {
	if !r.databaseReady {
		return doctorCheck{doctorFail, name, "optional; unavailable; database is not ready"}
	}
	if !r.configReady {
		return doctorCheck{doctorFail, name, "optional; unavailable; configuration could not be loaded"}
	}
	if r.legacyConfig && !provider.baseURLReady {
		return doctorCheck{doctorFail, name, "optional; unavailable; LLM base URL is invalid"}
	}
	modelName := "model"
	if r.legacyConfig {
		if name == "AI enhancement" {
			modelName = "chat model"
		} else {
			modelName = "embedding model"
		}
	}
	var missing []string
	if !provider.apiKeyReady {
		missing = append(missing, "API key")
	}
	if !provider.baseURLReady {
		missing = append(missing, "base URL")
	}
	if !provider.modelReady {
		missing = append(missing, modelName)
	}
	if len(missing) > 0 {
		return doctorCheck{doctorWarn, name, "optional; not configured; missing " + strings.Join(missing, ", ")}
	}
	if provider.apiStatus == doctorFail {
		return doctorCheck{doctorFail, name, "optional; unavailable; API check failed"}
	}
	if provider.apiStatus == doctorPass {
		return doctorCheck{doctorPass, name, "optional; configured; API reachable"}
	}
	return doctorCheck{doctorPass, name, "optional; configured; online not checked"}
}

func (r doctorReport) write(writer io.Writer) int {
	counts := map[doctorStatus]int{}
	fmt.Fprintln(writer, "ai-dev-logger doctor")
	for _, check := range r.checks {
		counts[check.Status]++
		fmt.Fprintf(writer, "[%-4s] %-16s %s\n", check.Status, check.Name, check.Detail)
	}
	// Capability rows summarize existing checks; do not count failures twice.
	if len(r.capabilities) > 0 {
		fmt.Fprintln(writer, "\nCapabilities:")
		for _, capability := range r.capabilities {
			fmt.Fprintf(writer, "[%-4s] %-16s %s\n", capability.Status, capability.Name, capability.Detail)
		}
	}
	fmt.Fprintf(
		writer,
		"\nSummary: %d passed, %d warning(s), %d failed, %d skipped\n",
		counts[doctorPass],
		counts[doctorWarn],
		counts[doctorFail],
		counts[doctorSkip],
	)
	return counts[doctorFail]
}
