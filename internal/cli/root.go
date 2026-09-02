package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-dev-logger/internal/buildinfo"
	appconfig "ai-dev-logger/internal/config"

	"github.com/spf13/cobra"
)

var dbPath string
var configPath string

var rootCmd = &cobra.Command{
	Use:           "ai-dev-logger [note text]",
	Short:         "AI development note CLI",
	Long:          "ai-dev-logger is a local CLI for collecting and searching development notes. Run it without note text to enter interactive mode.",
	Example:       "  adl \"fixed a SQLite lock issue #sqlite\"\n  adl\n  adl list",
	Version:       buildinfo.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractive(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), dbPath)
	},
	ValidArgsFunction: cobra.NoFileCompletions,
}

const quickAddCommandName = "__quick-add"

var quickAddCmd = &cobra.Command{
	Use:    quickAddCommandName + " <note text>",
	Hidden: true,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runQuickAdd(cmd.Context(), cmd.OutOrStdout(), dbPath, strings.Join(args, " "))
	},
	ValidArgsFunction: cobra.NoFileCompletions,
}

func Execute() {
	rootCmd.SetArgs(normalizeCommandLineArgs(os.Args[1:]))
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "SQLite database path")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.DefaultPath(), "Config file path")

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(embedCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(quickAddCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(semanticCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(versionCmd)
}

func normalizeCommandLineArgs(args []string) []string {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--help", argument == "-h", argument == "--version":
			return args
		case argument == "--db", argument == "--config":
			if index+1 >= len(args) {
				return args
			}
			index++
		case strings.HasPrefix(argument, "--db="), strings.HasPrefix(argument, "--config="):
			continue
		case argument == "--":
			if index+1 >= len(args) {
				return args
			}
			return insertQuickAddCommand(args, index)
		case strings.HasPrefix(argument, "-"):
			return args
		default:
			if isRootCommandName(argument) {
				return args
			}
			return insertQuickAddCommand(args, index)
		}
	}

	return args
}

func insertQuickAddCommand(args []string, index int) []string {
	normalized := make([]string, 0, len(args)+1)
	normalized = append(normalized, args[:index]...)
	normalized = append(normalized, quickAddCommandName)
	normalized = append(normalized, args[index:]...)
	return normalized
}

func isRootCommandName(value string) bool {
	if value == "help" || value == "__complete" || value == "__completeNoDesc" {
		return true
	}

	for _, command := range rootCmd.Commands() {
		if command.Name() == value {
			return true
		}
		for _, alias := range command.Aliases {
			if alias == value {
				return true
			}
		}
	}

	return false
}

func defaultDBPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "ai-dev-logger.db"
	}

	return filepath.Join(configDir, "ai-dev-logger", "notes.db")
}
