package cli

import (
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
	Short:         "本地开发笔记与 AI 整理工具",
	Long:          "ai-dev-logger（短命令 adl）用于记录和检索本地开发笔记。\n直接输入笔记可快速保存；不带参数时进入交互模式。\n本地记录和关键词搜索不需要 API Key，AI 整理和语义检索是可选功能。",
	Example:       "  adl \"解决了 SQLite 锁冲突 #sqlite\"\n  adl\n  adl list\n  adl search \"SQLite\"\n  adl setup\n  adl doctor",
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
	prepareHelp(rootCmd)
	rootCmd.SetArgs(normalizeCommandLineArgs(os.Args[1:]))
	if command, err := rootCmd.ExecuteC(); err != nil {
		writeCommandError(os.Stderr, command, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "SQLite 数据库路径")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", appconfig.DefaultPath(), "配置文件路径")

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(embedCmd)
	rootCmd.AddCommand(newEvalCommand())
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(newIngestCommand())
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
