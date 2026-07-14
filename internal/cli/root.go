package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var dbPath string

var rootCmd = &cobra.Command{
	Use:   "ai-dev-logger",
	Short: "AI 开发日志助手",
	Long:  "ai-dev-logger 是一个面向程序员本地使用的开发笔记 CLI。",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "SQLite database path")

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(searchCmd)
}

func defaultDBPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "ai-dev-logger.db"
	}

	return filepath.Join(configDir, "ai-dev-logger", "notes.db")
}
