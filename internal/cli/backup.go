package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var backupOutput string
var backupForce bool

type backupOptions struct {
	DBPath string
	Output string
	Force  bool
}

var backupCmd = &cobra.Command{
	Use:     "backup",
	Short:   "创建并校验完整 SQLite 备份",
	Long:    "备份包含笔记和向量，完成后校验完整性并显示 SHA-256。\n默认拒绝覆盖已有备份文件；API 配置和密钥不包含在数据库备份中。",
	Example: "  adl backup --output notes-backup.db",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup(cmd.Context(), cmd.OutOrStdout(), backupOptions{
			DBPath: dbPath,
			Output: backupOutput,
			Force:  backupForce,
		})
	},
}

func init() {
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "备份文件路径")
	backupCmd.Flags().BoolVar(&backupForce, "force", false, "覆盖已有备份文件")
	_ = backupCmd.MarkFlagRequired("output")
}

func runBackup(ctx context.Context, writer io.Writer, options backupOptions) error {
	outputPath := strings.TrimSpace(options.Output)
	if outputPath == "" {
		return fmt.Errorf("backup output path is empty, pass --output")
	}

	sourcePath := strings.TrimSpace(sqliteDatabaseFilePath(options.DBPath))
	if sourcePath == "" {
		return fmt.Errorf("SQLite database path is empty")
	}
	if pathsReferToSameFile(sourcePath, outputPath) {
		return fmt.Errorf("backup output must not be the source SQLite database: %s", outputPath)
	}
	if err := validateBackupSource(sourcePath); err != nil {
		return err
	}

	targetExists, err := inspectBackupOutput(outputPath, options.Force)
	if err != nil {
		return err
	}
	tempPath, err := prepareBackupTempPath(outputPath)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	backupErr := db.Backup(ctx, tempPath)
	closeErr := db.Close()
	if backupErr != nil {
		return backupErr
	}
	if closeErr != nil {
		return fmt.Errorf("close source database after backup: %w", closeErr)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return fmt.Errorf("protect backup file permissions: %w", err)
	}

	hash, size, err := sha256File(tempPath)
	if err != nil {
		return err
	}
	if err := installBackupFile(tempPath, outputPath, targetExists); err != nil {
		return err
	}

	fmt.Fprintf(writer, "backup created: %s\n", outputPath)
	fmt.Fprintf(writer, "size: %d bytes\n", size)
	fmt.Fprintf(writer, "sha256: %s\n", hash)
	fmt.Fprintln(writer, "integrity: ok")
	return nil
}

func validateBackupSource(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("SQLite database does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("inspect SQLite database: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("SQLite database path is a directory: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("SQLite database path is not a regular file: %s", path)
	}
	return nil
}

func inspectBackupOutput(path string, force bool) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect backup output: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("backup output path is a directory: %s", path)
	}
	if !force {
		return true, fmt.Errorf("backup file already exists: %s (use --force to replace)", path)
	}
	return true, nil
}

func prepareBackupTempPath(outputPath string) (string, error) {
	directory := filepath.Dir(outputPath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	tempFile, err := os.CreateTemp(directory, "."+filepath.Base(outputPath)+".tmp-*.db")
	if err != nil {
		return "", fmt.Errorf("prepare temporary backup path: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("close temporary backup placeholder: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return "", fmt.Errorf("prepare temporary backup path: %w", err)
	}
	return tempPath, nil
}

func sha256File(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open backup for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, fmt.Errorf("calculate backup checksum: %w", err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), size, nil
}

func installBackupFile(tempPath string, outputPath string, targetExists bool) error {
	if !targetExists {
		if err := os.Rename(tempPath, outputPath); err != nil {
			return fmt.Errorf("install backup file: %w", err)
		}
		return nil
	}

	// Unix can replace the target directly. Windows uses a recoverable swap.
	if err := os.Rename(tempPath, outputPath); err == nil {
		return nil
	}

	directory := filepath.Dir(outputPath)
	previousFile, err := os.CreateTemp(directory, "."+filepath.Base(outputPath)+".previous-*")
	if err != nil {
		return fmt.Errorf("prepare previous backup path: %w", err)
	}
	previousPath := previousFile.Name()
	if err := previousFile.Close(); err != nil {
		os.Remove(previousPath)
		return fmt.Errorf("close previous backup placeholder: %w", err)
	}
	if err := os.Remove(previousPath); err != nil {
		return fmt.Errorf("prepare previous backup path: %w", err)
	}
	if err := os.Rename(outputPath, previousPath); err != nil {
		return fmt.Errorf("preserve existing backup file: %w", err)
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		restoreErr := os.Rename(previousPath, outputPath)
		if restoreErr != nil {
			return fmt.Errorf("replace backup file: %v; restore previous backup: %w", err, restoreErr)
		}
		return fmt.Errorf("replace backup file: %w (previous backup restored)", err)
	}
	if err := os.Remove(previousPath); err != nil {
		return fmt.Errorf("backup replaced, but remove previous file %s: %w", previousPath, err)
	}
	return nil
}
