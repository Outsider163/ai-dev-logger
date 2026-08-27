package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	sqlite "modernc.org/sqlite"
)

const restorePageBatchSize int32 = 256

type DatabaseInfo struct {
	SchemaVersion int
	Notes         int
	Embeddings    int
}

type sqliteOnlineRestorer interface {
	NewRestore(string) (*sqlite.Backup, error)
}

// InspectDatabase verifies that path is a compatible ai-dev-logger database.
func InspectDatabase(ctx context.Context, path string) (DatabaseInfo, error) {
	if err := VerifyDatabase(ctx, path); err != nil {
		return DatabaseInfo{}, err
	}

	db, err := openReadOnlyDatabase(path)
	if err != nil {
		return DatabaseInfo{}, err
	}
	defer db.Close()

	var info DatabaseInfo
	if err := db.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0)
FROM schema_migrations
`).Scan(&info.SchemaVersion); err != nil {
		return DatabaseInfo{}, fmt.Errorf("read ai-dev-logger schema version: %w", err)
	}
	if info.SchemaVersion <= 0 {
		return DatabaseInfo{}, fmt.Errorf("database has no supported ai-dev-logger schema migration")
	}
	if info.SchemaVersion > currentSchemaVersion {
		return DatabaseInfo{}, fmt.Errorf(
			"database schema version %d is newer than supported version %d",
			info.SchemaVersion,
			currentSchemaVersion,
		)
	}

	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes`).Scan(&info.Notes); err != nil {
		return DatabaseInfo{}, fmt.Errorf("count notes in database: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_embeddings`).Scan(&info.Embeddings); err != nil {
		return DatabaseInfo{}, fmt.Errorf("count embeddings in database: %w", err)
	}
	if err := verifyForeignKeys(ctx, db); err != nil {
		return DatabaseInfo{}, err
	}
	return info, nil
}

// Restore replaces the current database contents through SQLite's online backup API.
func (s *Store) Restore(ctx context.Context, sourcePath string) error {
	if _, err := InspectDatabase(ctx, sourcePath); err != nil {
		return fmt.Errorf("inspect restore source: %w", err)
	}

	sourceDSN, err := sqliteReadOnlyDSN(sourcePath)
	if err != nil {
		return err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve destination database connection: %w", err)
	}
	defer connection.Close()

	if err := connection.Raw(func(driverConnection any) error {
		restorer, ok := driverConnection.(sqliteOnlineRestorer)
		if !ok {
			return fmt.Errorf("SQLite driver does not support online restore")
		}
		backup, err := restorer.NewRestore(sourceDSN)
		if err != nil {
			return fmt.Errorf("start SQLite online restore: %w", err)
		}
		return runOnlineRestore(ctx, backup)
	}); err != nil {
		return fmt.Errorf("restore SQLite database: %w", err)
	}
	return nil
}

func runOnlineRestore(ctx context.Context, backup *sqlite.Backup) (returnErr error) {
	finished := false
	defer func() {
		if finished {
			return
		}
		if err := backup.Finish(); returnErr == nil && err != nil {
			returnErr = fmt.Errorf("finish canceled SQLite restore: %w", err)
		}
	}()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := backup.Step(restorePageBatchSize)
		if err != nil {
			return fmt.Errorf("copy SQLite restore pages: %w", err)
		}
		if !more {
			break
		}
	}

	finishErr := backup.Finish()
	finished = true
	if finishErr != nil {
		return fmt.Errorf("finish SQLite restore: %w", finishErr)
	}
	return nil
}

func openReadOnlyDatabase(path string) (*sql.DB, error) {
	dsn, err := sqliteReadOnlyDSN(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func sqliteReadOnlyDSN(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("database path is empty")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	urlPath := filepath.ToSlash(absolutePath)
	if filepath.VolumeName(absolutePath) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}

	fileURL := &url.URL{
		Scheme: "file",
		Path:   urlPath,
	}
	parameters := fileURL.Query()
	parameters.Set("mode", "ro")
	parameters.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMilliseconds))
	parameters.Add("_pragma", "foreign_keys(ON)")
	fileURL.RawQuery = parameters.Encode()
	return fileURL.String(), nil
}

func verifyForeignKeys(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run SQLite foreign key check: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return fmt.Errorf("read SQLite foreign key result: %w", err)
		}
		rowDescription := "unknown"
		if rowID.Valid {
			rowDescription = fmt.Sprintf("%d", rowID.Int64)
		}
		return fmt.Errorf(
			"SQLite foreign key check failed: table=%s rowid=%s parent=%s constraint=%d",
			table,
			rowDescription,
			parent,
			foreignKeyID,
		)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read SQLite foreign key results: %w", err)
	}
	return nil
}
