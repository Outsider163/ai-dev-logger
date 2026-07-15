package cli

import (
	appconfig "ai-dev-logger/internal/config"

	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var dbPath string
var configPath string

var rootCmd = &cobra.Command{
	Use:           "ai-dev-logger",
	Short:         "AI development note CLI",
	Long:          "ai-dev-logger is a local CLI for collecting and searching development notes.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "SQLite database path")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.DefaultPath(), "Config file path")

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(embedCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(semanticCmd)
}

func defaultDBPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "ai-dev-logger.db"
	}

	return filepath.Join(configDir, "ai-dev-logger", "notes.db")
}
