package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Backup creates a consistent SQLite snapshot and verifies it before returning.
func (s *Store) Backup(ctx context.Context, destination string) error {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return fmt.Errorf("backup destination is empty")
	}

	absolutePath, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve backup destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	info, statErr := os.Stat(absolutePath)
	if statErr == nil {
		if info.IsDir() {
			return fmt.Errorf("backup destination is a directory: %s", destination)
		}
		return fmt.Errorf("backup destination already exists: %s", destination)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", statErr)
	}

	if _, err := s.db.ExecContext(ctx, `VACUUM main INTO ?`, absolutePath); err != nil {
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	if err := VerifyDatabase(ctx, absolutePath); err != nil {
		return fmt.Errorf("verify SQLite backup: %w", err)
	}
	return nil
}

// VerifyDatabase runs SQLite's full integrity check without applying migrations.
func VerifyDatabase(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("database path is empty")
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("database file does not exist: %s", path)
	}
	if err != nil {
		return fmt.Errorf("inspect database file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("database path is a directory: %s", path)
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return fmt.Errorf("open database for integrity check: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("run SQLite integrity check: %w", err)
	}
	defer rows.Close()

	var problems []string
	resultCount := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("read SQLite integrity result: %w", err)
		}
		resultCount++
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			problems = append(problems, result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read SQLite integrity results: %w", err)
	}
	if resultCount == 0 {
		return fmt.Errorf("SQLite integrity check returned no result")
	}
	if len(problems) > 0 {
		return fmt.Errorf("SQLite integrity check failed: %s", strings.Join(problems, "; "))
	}
	return nil
}
