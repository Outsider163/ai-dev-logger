package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectDatabaseReturnsProjectInformationReadOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "directory with spaces", "notes.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "inspect", Body: "database information"})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "test-model",
		Text:   NoteEmbeddingText(note),
		Vector: []float64{0.1, 0.2},
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := InspectDatabase(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if info.SchemaVersion != currentSchemaVersion || info.Notes != 1 || info.Embeddings != 1 {
		t.Fatalf("unexpected database info: %#v", info)
	}

	readOnly, err := openReadOnlyDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if _, err := readOnly.ExecContext(ctx, `DELETE FROM notes`); err == nil {
		t.Fatal("expected read-only database to reject writes")
	}
}

func TestInspectDatabaseRejectsNonProjectAndNewerSchema(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	nonProjectPath := filepath.Join(tempDir, "other.db")
	nonProject, err := sql.Open("sqlite", nonProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nonProject.ExecContext(ctx, `CREATE TABLE example (id INTEGER PRIMARY KEY)`); err != nil {
		nonProject.Close()
		t.Fatal(err)
	}
	if err := nonProject.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDatabase(ctx, nonProjectPath); err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected non-project database error, got %v", err)
	}

	newerPath := filepath.Join(tempDir, "newer.db")
	newer, err := Open(newerPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := newer.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", newerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (999, '2026-08-27T00:00:00Z')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDatabase(ctx, newerPath); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected newer schema error, got %v", err)
	}
}

func TestInspectDatabaseRejectsForeignKeyViolation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "invalid-foreign-key.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.ExecContext(ctx, `
INSERT INTO note_embeddings (
    note_id, model, dimensions, vector_json, content_hash, created_at, updated_at
) VALUES (
    999, 'test-model', 1, '[0.1]', 'hash', '2026-08-27T00:00:00Z', '2026-08-27T00:00:00Z'
)
`)
	if err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := InspectDatabase(ctx, path); err == nil || !strings.Contains(err.Error(), "foreign key check failed") {
		t.Fatalf("expected foreign key error, got %v", err)
	}
}

func TestRestoreReplacesDatabaseWithSourceSnapshot(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := source.CreateNote(ctx, CreateNoteInput{Title: "restored first", Body: "source body"})
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	if _, err := source.CreateNote(ctx, CreateNoteInput{Title: "restored second", Body: "another source note"}); err != nil {
		source.Close()
		t.Fatal(err)
	}
	if _, err := source.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: first.ID,
		Model:  "source-model",
		Text:   NoteEmbeddingText(first),
		Vector: []float64{0.4, 0.5, 0.6},
	}); err != nil {
		source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	sourceBytesBefore, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sourceHashBefore := sha256.Sum256(sourceBytesBefore)

	targetPath := filepath.Join(tempDir, "target.db")
	target, err := Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if _, err := target.CreateNote(ctx, CreateNoteInput{Title: "old target", Body: "must disappear"}); err != nil {
		t.Fatal(err)
	}

	if err := target.Restore(ctx, sourcePath); err != nil {
		t.Fatal(err)
	}
	notes, err := target.ListAllNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Title != "restored first" || notes[1].Title != "restored second" {
		t.Fatalf("unexpected restored notes: %#v", notes)
	}
	embedding, err := target.GetEmbedding(ctx, first.ID, "source-model")
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Vector) != 3 || embedding.Vector[2] != 0.6 {
		t.Fatalf("unexpected restored embedding: %#v", embedding)
	}

	sourceBytesAfter, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(sourceBytesAfter) != sourceHashBefore {
		t.Fatal("restore source was modified")
	}
}

func TestRestoreInvalidOrCanceledSourceKeepsTarget(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "target.db")
	target, err := Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	oldNote, err := target.CreateNote(ctx, CreateNoteInput{Title: "keep target", Body: "original"})
	if err != nil {
		t.Fatal(err)
	}

	invalidPath := filepath.Join(tempDir, "invalid.db")
	if err := os.WriteFile(invalidPath, []byte("not SQLite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := target.Restore(ctx, invalidPath); err == nil {
		t.Fatal("expected invalid source error")
	}
	if note, err := target.GetNote(ctx, oldNote.ID); err != nil || note.Title != oldNote.Title {
		t.Fatalf("target changed after invalid restore: note=%#v err=%v", note, err)
	}

	sourcePath := filepath.Join(tempDir, "source.db")
	source, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.CreateNote(ctx, CreateNoteInput{Title: "new source", Body: "new"}); err != nil {
		source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	canceledContext, cancel := context.WithCancel(ctx)
	cancel()
	if err := target.Restore(canceledContext, sourcePath); err == nil {
		t.Fatal("expected canceled restore error")
	}
	if note, err := target.GetNote(ctx, oldNote.ID); err != nil || note.Title != oldNote.Title {
		t.Fatalf("target changed after canceled restore: note=%#v err=%v", note, err)
	}
}
