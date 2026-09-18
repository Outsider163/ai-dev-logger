package store

import (
	"context"
	"fmt"
	"time"
)

const currentSchemaVersion = 3

type schemaMigration struct {
	Version int
	SQL     string
}

var schemaMigrations = []schemaMigration{
	{
		Version: 1,
		SQL: `
CREATE TABLE IF NOT EXISTS notes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	body TEXT NOT NULL,
	tags_json TEXT NOT NULL DEFAULT '[]',
	summary TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at);

CREATE TABLE IF NOT EXISTS note_embeddings (
	note_id INTEGER NOT NULL,
	model TEXT NOT NULL,
	dimensions INTEGER NOT NULL,
	vector_json TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (note_id, model),
	FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_note_embeddings_model ON note_embeddings(model);
`,
	},
	{Version: 2, SQL: `
CREATE TABLE note_chunks (
 note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
 chunk_index INTEGER NOT NULL CHECK(chunk_index >= 0),
 content TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 PRIMARY KEY(note_id,chunk_index)
);
ALTER TABLE note_embeddings RENAME TO legacy_note_embeddings;
DROP INDEX idx_note_embeddings_model;
CREATE TABLE note_embeddings (
 note_id INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
 chunk_index INTEGER NOT NULL DEFAULT 0 CHECK(chunk_index >= 0),
 model TEXT NOT NULL,
 dimensions INTEGER NOT NULL,
 vector_json TEXT NOT NULL,
 content_hash TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 PRIMARY KEY(note_id,chunk_index,model)
);
INSERT INTO note_embeddings(note_id,model,dimensions,vector_json,content_hash,created_at,updated_at)
SELECT note_id,model,dimensions,vector_json,content_hash,created_at,updated_at FROM legacy_note_embeddings;
DROP TABLE legacy_note_embeddings;
CREATE INDEX idx_note_embeddings_model ON note_embeddings(model);
`},
	{Version: 3, SQL: `
CREATE TABLE note_sources (
 path TEXT PRIMARY KEY,
 note_id INTEGER NOT NULL UNIQUE REFERENCES notes(id) ON DELETE CASCADE,
 body_hash TEXT NOT NULL
);
`},
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version INTEGER PRIMARY KEY,
	applied_at TEXT NOT NULL
)
`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}

	var version int
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0)
FROM schema_migrations
`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, currentSchemaVersion)
	}

	for _, migration := range schemaMigrations {
		if migration.Version <= version {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply schema migration %d: %w", migration.Version, err)
		}
		if migration.Version == 2 {
			if err := backfillChunks(ctx, tx); err != nil {
				return fmt.Errorf("backfill note chunks: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO schema_migrations (version, applied_at)
VALUES (?, ?)
`, migration.Version, formatTime(time.Now().UTC())); err != nil {
			return fmt.Errorf("record schema migration %d: %w", migration.Version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}
	return nil
}
