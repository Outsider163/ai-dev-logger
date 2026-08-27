package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var restoreInput string
var restoreDryRun bool
var restoreYes bool

type restoreOptions struct {
	DBPath string
	Input  string
	DryRun bool
	Yes    bool
	Now    time.Time
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Verify and restore a complete SQLite backup",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore(cmd.Context(), cmd.OutOrStdout(), restoreOptions{
			DBPath: dbPath,
			Input:  restoreInput,
			DryRun: restoreDryRun,
			Yes:    restoreYes,
		})
	},
}

func init() {
	restoreCmd.Flags().StringVarP(&restoreInput, "input", "i", "", "SQLite backup file path")
	restoreCmd.Flags().BoolVar(&restoreDryRun, "dry-run", false, "Verify and describe the restore without changing files")
	restoreCmd.Flags().BoolVarP(&restoreYes, "yes", "y", false, "Confirm replacing the target database")
	_ = restoreCmd.MarkFlagRequired("input")
}

func runRestore(ctx context.Context, writer io.Writer, options restoreOptions) error {
	inputPath := strings.TrimSpace(options.Input)
	if inputPath == "" {
		return fmt.Errorf("restore input path is empty, pass --input")
	}
	targetPath := strings.TrimSpace(sqliteDatabaseFilePath(options.DBPath))
	if targetPath == "" || targetPath == ":memory:" || strings.HasPrefix(targetPath, "file:") {
		return fmt.Errorf("restore requires a filesystem SQLite target path")
	}
	if pathsReferToSameFile(inputPath, targetPath) {
		return fmt.Errorf("restore input must not be the target SQLite database: %s", inputPath)
	}
	if err := validateRestoreInput(inputPath); err != nil {
		return err
	}

	sourceInfo, err := store.InspectDatabase(ctx, inputPath)
	if err != nil {
		return fmt.Errorf("inspect restore input: %w", err)
	}
	sourceHash, sourceSize, err := sha256File(inputPath)
	if err != nil {
		return err
	}
	targetExists, err := inspectRestoreTarget(targetPath)
	if err != nil {
		return err
	}
	var targetInfo store.DatabaseInfo
	if targetExists {
		targetInfo, err = store.InspectDatabase(ctx, targetPath)
		if err != nil {
			return fmt.Errorf("inspect current target database: %w", err)
		}
	}

	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var safetyPath string
	if targetExists {
		safetyPath, err = nextRestoreSafetyPath(targetPath, now)
		if err != nil {
			return err
		}
	}

	if options.DryRun {
		writeRestoreSourceSummary(writer, inputPath, sourceHash, sourceSize, sourceInfo)
		fmt.Fprintf(writer, "target database: %s\n", targetPath)
		if targetExists {
			fmt.Fprintf(writer, "target schema version: %d\n", targetInfo.SchemaVersion)
			fmt.Fprintf(writer, "target notes: %d\n", targetInfo.Notes)
			fmt.Fprintf(writer, "target embeddings: %d\n", targetInfo.Embeddings)
			fmt.Fprintf(writer, "safety backup: would create %s\n", safetyPath)
		} else {
			fmt.Fprintln(writer, "safety backup: not needed (target does not exist)")
		}
		fmt.Fprintln(writer, "dry run: no files changed")
		return nil
	}
	if !options.Yes {
		return fmt.Errorf("restore replaces the target database; run --dry-run first, then rerun with --yes")
	}

	target, err := store.Open(options.DBPath)
	if err != nil {
		return fmt.Errorf("open target database: %w", err)
	}
	restoreCompleted := false
	if !targetExists {
		defer func() {
			if !restoreCompleted {
				removeSQLiteDatabaseFiles(targetPath)
			}
		}()
	}

	var safetyHash string
	if targetExists {
		if err := target.Backup(ctx, safetyPath); err != nil {
			target.Close()
			os.Remove(safetyPath)
			return fmt.Errorf("create pre-restore safety backup: %w", err)
		}
		if err := os.Chmod(safetyPath, 0o600); err != nil {
			target.Close()
			return fmt.Errorf("protect pre-restore safety backup permissions: %w", err)
		}
		safetyHash, _, err = sha256File(safetyPath)
		if err != nil {
			target.Close()
			return fmt.Errorf("hash pre-restore safety backup: %w", err)
		}
	}
	if err := verifyRestoreInputUnchanged(inputPath, sourceHash, sourceSize); err != nil {
		target.Close()
		return restoreVerificationError(err, safetyPath)
	}

	if err := target.Restore(ctx, inputPath); err != nil {
		target.Close()
		if safetyPath != "" {
			return fmt.Errorf("restore target database: %w; safety backup preserved at %s", err, safetyPath)
		}
		return fmt.Errorf("restore target database: %w", err)
	}
	restoreCompleted = true
	if err := target.Close(); err != nil {
		return fmt.Errorf("close restored database: %w", err)
	}

	// Reopen once so any supported older schema is migrated before final verification.
	reopened, err := store.Open(options.DBPath)
	if err != nil {
		return restoreVerificationError(fmt.Errorf("reopen restored database: %w", err), safetyPath)
	}
	if err := reopened.Close(); err != nil {
		return restoreVerificationError(fmt.Errorf("close migrated restored database: %w", err), safetyPath)
	}
	restoredInfo, err := store.InspectDatabase(ctx, targetPath)
	if err != nil {
		return restoreVerificationError(fmt.Errorf("verify restored database: %w", err), safetyPath)
	}
	if restoredInfo.Notes != sourceInfo.Notes || restoredInfo.Embeddings != sourceInfo.Embeddings {
		return restoreVerificationError(fmt.Errorf(
			"restored database counts changed: source has %d note(s) and %d embedding(s), target has %d note(s) and %d embedding(s)",
			sourceInfo.Notes,
			sourceInfo.Embeddings,
			restoredInfo.Notes,
			restoredInfo.Embeddings,
		), safetyPath)
	}
	if err := verifyRestoreInputUnchanged(inputPath, sourceHash, sourceSize); err != nil {
		return restoreVerificationError(err, safetyPath)
	}

	writeRestoreSourceSummary(writer, inputPath, sourceHash, sourceSize, sourceInfo)
	if safetyPath != "" {
		fmt.Fprintf(writer, "safety backup created: %s\n", safetyPath)
		fmt.Fprintf(writer, "safety backup sha256: %s\n", safetyHash)
	}
	fmt.Fprintf(writer, "restored database: %s\n", targetPath)
	fmt.Fprintln(writer, "integrity: ok")
	return nil
}

func validateRestoreInput(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("restore input does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("inspect restore input: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("restore input is a directory: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("restore input is not a regular file: %s", path)
	}
	return nil
}

func inspectRestoreTarget(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect restore target: %w", err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("restore target is a directory: %s", path)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("restore target is not a regular file: %s", path)
	}
	return true, nil
}

func nextRestoreSafetyPath(targetPath string, now time.Time) (string, error) {
	directory := filepath.Dir(targetPath)
	extension := filepath.Ext(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), extension)
	stamp := now.UTC().Format("20060102T150405Z")

	for attempt := 0; attempt < 1000; attempt++ {
		suffix := ""
		if attempt > 0 {
			suffix = fmt.Sprintf("-%d", attempt)
		}
		candidate := filepath.Join(directory, base+".pre-restore-"+stamp+suffix+extension)
		_, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect pre-restore safety backup path: %w", err)
		}
	}
	return "", fmt.Errorf("could not find an unused pre-restore safety backup path")
}

func writeRestoreSourceSummary(writer io.Writer, path string, hash string, size int64, info store.DatabaseInfo) {
	fmt.Fprintf(writer, "restore source verified: %s\n", path)
	fmt.Fprintf(writer, "schema version: %d\n", info.SchemaVersion)
	fmt.Fprintf(writer, "notes: %d\n", info.Notes)
	fmt.Fprintf(writer, "embeddings: %d\n", info.Embeddings)
	fmt.Fprintf(writer, "size: %d bytes\n", size)
	fmt.Fprintf(writer, "sha256: %s\n", hash)
}

func restoreVerificationError(err error, safetyPath string) error {
	if safetyPath == "" {
		return err
	}
	return fmt.Errorf("%w; pre-restore safety backup preserved at %s", err, safetyPath)
}

func verifyRestoreInputUnchanged(path string, expectedHash string, expectedSize int64) error {
	hash, size, err := sha256File(path)
	if err != nil {
		return fmt.Errorf("recheck restore input: %w", err)
	}
	if hash != expectedHash || size != expectedSize {
		return fmt.Errorf("restore input changed during validation or restore: %s", path)
	}
	return nil
}

func removeSQLiteDatabaseFiles(path string) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(path + suffix)
	}
}
