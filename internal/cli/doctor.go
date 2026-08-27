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
	checks []doctorCheck
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose local configuration and service readiness",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if doctorTimeout <= 0 {
			return fmt.Errorf("timeout must be greater than zero")
		}

		return runDoctor(cmd.Context(), cmd.OutOrStdout(), doctorOptions{
			ConfigPath: configPath,
			DBPath:     dbPath,
			Online:     doctorOnline,
			Timeout:    doctorTimeout,
		})
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorOnline, "online", false, "Call chat and embedding APIs")
	doctorCmd.Flags().DurationVar(&doctorTimeout, "timeout", defaultDoctorTimeout, "Timeout for each online check")
}

func runDoctor(ctx context.Context, writer io.Writer, options doctorOptions) error {
	report := doctorReport{}
	cfg, configReady, configCheck := inspectConfig(options.ConfigPath)
	report.add(configCheck)
	report.add(inspectDatabase(options.DBPath))

	apiKeyReady := false
	baseURLReady := false
	chatModelReady := false
	embeddingModelReady := false

	if !configReady {
		report.addSkippedConfigChecks()
	} else {
		apiKey, apiKeySource := appconfig.ResolveAPIKey(cfg.LLM.APIKey)
		apiKeyReady = apiKey != ""
		if apiKeyReady {
			report.add(doctorCheck{doctorPass, "API key", "configured via " + apiKeySource})
		} else {
			report.add(doctorCheck{
				Status: doctorFail,
				Name:   "API key",
				Detail: "missing; set " + appconfig.EnvAPIKey + " or run config set --api-key",
			})
		}

		baseURL := strings.TrimSpace(cfg.LLM.BaseURL)
		if err := validateHTTPBaseURL(baseURL); err != nil {
			report.add(doctorCheck{doctorFail, "LLM base URL", err.Error()})
		} else {
			baseURLReady = true
			report.add(doctorCheck{doctorPass, "LLM base URL", baseURL})
		}

		chatModel := strings.TrimSpace(cfg.LLM.Model)
		chatModelReady = chatModel != ""
		if chatModelReady {
			report.add(doctorCheck{doctorPass, "chat model", chatModel})
		} else {
			report.add(doctorCheck{doctorFail, "chat model", "missing; run config set --model"})
		}

		embeddingModel := strings.TrimSpace(cfg.LLM.EmbeddingModel)
		embeddingModelReady = embeddingModel != ""
		if embeddingModelReady {
			report.add(doctorCheck{doctorPass, "embedding model", embeddingModel})
		} else {
			report.add(doctorCheck{doctorFail, "embedding model", "missing; run config set --embedding-model"})
		}
	}

	if !options.Online {
		report.add(doctorCheck{
			Status: doctorWarn,
			Name:   "online probes",
			Detail: "not requested; run ai-dev-logger doctor --online",
		})
	} else if !configReady {
		report.add(doctorCheck{doctorSkip, "chat API", "configuration could not be loaded"})
		report.add(doctorCheck{doctorSkip, "embedding API", "configuration could not be loaded"})
	} else {
		client := llm.NewClient(cfg.LLM)
		if apiKeyReady && baseURLReady && chatModelReady {
			report.add(probeChat(ctx, client, options.Timeout))
		} else {
			report.add(doctorCheck{doctorSkip, "chat API", "required settings are not ready"})
		}
		if apiKeyReady && baseURLReady && embeddingModelReady {
			report.add(probeEmbedding(ctx, client, options.Timeout))
		} else {
			report.add(doctorCheck{doctorSkip, "embedding API", "required settings are not ready"})
		}
	}

	failed := report.write(writer)
	if failed > 0 {
		return fmt.Errorf("doctor found %d failed check(s)", failed)
	}
	return nil
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
		return fmt.Errorf("missing; run config set --base-url")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid HTTP(S) URL %q; run config set --base-url", value)
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

func (r *doctorReport) addSkippedConfigChecks() {
	for _, name := range []string{"API key", "LLM base URL", "chat model", "embedding model"} {
		r.add(doctorCheck{doctorSkip, name, "configuration could not be loaded"})
	}
}

func (r doctorReport) write(writer io.Writer) int {
	counts := map[doctorStatus]int{}
	fmt.Fprintln(writer, "ai-dev-logger doctor")
	for _, check := range r.checks {
		counts[check.Status]++
		fmt.Fprintf(writer, "[%-4s] %-16s %s\n", check.Status, check.Name, check.Detail)
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
