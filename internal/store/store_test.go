package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenConfiguresSQLiteAndRecordsSchemaVersion(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var foreignKeys int
	if err := db.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign keys to be enabled, got %d", foreignKeys)
	}

	var busyTimeout int
	if err := db.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != sqliteBusyTimeoutMilliseconds {
		t.Fatalf("expected busy timeout %d, got %d", sqliteBusyTimeoutMilliseconds, busyTimeout)
	}

	var schemaVersion int
	if err := db.db.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version), 0)
FROM schema_migrations
`).Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != currentSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", currentSchemaVersion, schemaVersion)
	}
}

func TestMigrationPreservesExistingV1Notes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	rawDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.ExecContext(ctx, schemaMigrations[0].SQL); err != nil {
		rawDB.Close()
		t.Fatal(err)
	}
	if _, err := rawDB.ExecContext(ctx, `
INSERT INTO notes (id, title, body, tags_json, summary, created_at, updated_at)
VALUES (1, 'Legacy note', 'keep this body', '[]', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
`); err != nil {
		rawDB.Close()
		t.Fatal(err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.GetNote(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if note.Title != "Legacy note" || note.Body != "keep this body" {
		t.Fatalf("migration changed existing note: %#v", note)
	}
}

func TestUpdateNoteRollsBackWhenEmbeddingCleanupFails(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "Before", Body: "old body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "model-a",
		Text:   note.Body,
		Vector: []float64{0.1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `
CREATE TRIGGER reject_embedding_delete
BEFORE DELETE ON note_embeddings
BEGIN
	SELECT RAISE(ABORT, 'forced embedding cleanup failure');
END
`); err != nil {
		t.Fatal(err)
	}

	updatedBody := "new body"
	if _, err := db.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, Body: &updatedBody}); err == nil {
		t.Fatal("expected update to fail when embedding cleanup fails")
	}

	got, err := db.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != note.Body {
		t.Fatalf("expected note update to roll back, got body %q", got.Body)
	}
	if _, err := db.GetEmbedding(ctx, note.ID, "model-a"); err != nil {
		t.Fatalf("expected embedding to remain after rollback, got %v", err)
	}
}

func TestDeleteNoteCascadeRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "Keep", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "model-a",
		Text:   note.Body,
		Vector: []float64{0.1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `
CREATE TRIGGER reject_cascade_delete
BEFORE DELETE ON note_embeddings
BEGIN
	SELECT RAISE(ABORT, 'forced cascade failure');
END
`); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteNote(ctx, note.ID); err == nil {
		t.Fatal("expected delete to fail when cascading embedding delete fails")
	}
	if _, err := db.GetNote(ctx, note.ID); err != nil {
		t.Fatalf("expected note to remain after failed cascade, got %v", err)
	}
	if _, err := db.GetEmbedding(ctx, note.ID, "model-a"); err != nil {
		t.Fatalf("expected embedding to remain after failed cascade, got %v", err)
	}
}

func TestForeignKeyRejectsOrphanEmbedding(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.db.ExecContext(ctx, `
INSERT INTO note_embeddings (
	note_id, model, dimensions, vector_json, content_hash, created_at, updated_at
)
VALUES (404, 'model-a', 1, '[0.1]', 'hash', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
`)
	if err == nil {
		t.Fatal("expected foreign key to reject an orphan embedding")
	}

	var embeddings int
	if err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM note_embeddings").Scan(&embeddings); err != nil {
		t.Fatal(err)
	}
	if embeddings != 0 {
		t.Fatalf("expected no orphan embeddings, got %d", embeddings)
	}
}
