package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncUpdatesStableIDAndInvalidatesDerivedData(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	input := SyncNoteInput{Path: filepath.Join(t.TempDir(), "note.md"), Title: "note", Body: "original", Tags: []string{"go"}}
	result, err := db.SyncNotes(ctx, []SyncNoteInput{input}, false)
	if err != nil || result.Created != 1 {
		t.Fatalf("create: %+v %v", result, err)
	}
	notes, err := db.ListAllNotes(ctx)
	if err != nil || len(notes) != 1 {
		t.Fatal(notes, err)
	}
	id := notes[0].ID
	summary := "old summary"
	note, err := db.UpdateNote(ctx, UpdateNoteInput{ID: id, Summary: &summary})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{NoteID: id, Model: "m", Text: NoteEmbeddingText(note), Vector: []float64{1, 0}}); err != nil {
		t.Fatal(err)
	}
	result, err = db.SyncNotes(ctx, []SyncNoteInput{input}, false)
	if err != nil || result.Unchanged != 1 {
		t.Fatalf("unchanged: %+v %v", result, err)
	}
	if _, err := db.GetEmbedding(ctx, id, "m"); err != nil {
		t.Fatal("unchanged sync removed vector", err)
	}
	input.Body, input.Title, input.Tags = strings.Repeat("updated ", 250), "other title", []string{"other"}
	result, err = db.SyncNotes(ctx, []SyncNoteInput{input}, true)
	if err != nil || result.Updated != 1 {
		t.Fatalf("preview: %+v %v", result, err)
	}
	preview, err := db.GetNote(ctx, id)
	if err != nil || preview.Body != "original" {
		t.Fatal("preview changed note", err)
	}
	if _, err := db.GetEmbedding(ctx, id, "m"); err != nil {
		t.Fatal("preview removed vector", err)
	}
	result, err = db.SyncNotes(ctx, []SyncNoteInput{input}, false)
	if err != nil || result.Updated != 1 {
		t.Fatalf("update: %+v %v", result, err)
	}
	updated, err := db.GetNote(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != input.Body || updated.Summary != "" || updated.Title != "note" || updated.Tags[0] != "go" || !updated.CreatedAt.Equal(note.CreatedAt) {
		t.Fatalf("unexpected note: %+v", updated)
	}
	assertChunks(t, db, updated)
	embeddings, err := db.ListEmbeddings(ctx, "m")
	if err != nil || len(embeddings) != 0 {
		t.Fatalf("stale vectors retained: %v %v", embeddings, err)
	}
}

func TestSyncConflictRollsBackBatchAndCanBeReconciled(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	dir := t.TempDir()
	input := SyncNoteInput{Path: filepath.Join(dir, "tracked.md"), Title: "tracked", Body: "original"}
	if _, err := db.SyncNotes(ctx, []SyncNoteInput{input}, false); err != nil {
		t.Fatal(err)
	}
	notes, _ := db.ListAllNotes(ctx)
	local := "local edit"
	if _, err := db.UpdateNote(ctx, UpdateNoteInput{ID: notes[0].ID, Body: &local}); err != nil {
		t.Fatal(err)
	}
	input.Body = "file edit"
	newInput := SyncNoteInput{Path: filepath.Join(dir, "new.md"), Title: "new", Body: "new"}
	if _, err := db.SyncNotes(ctx, []SyncNoteInput{newInput, input}, false); err == nil || !strings.Contains(err.Error(), "sync conflict") {
		t.Fatalf("conflict = %v", err)
	}
	after, _ := db.ListAllNotes(ctx)
	if len(after) != 1 || after[0].Body != local {
		t.Fatalf("partial batch persisted: %+v", after)
	}
	input.Body = local
	if result, err := db.SyncNotes(ctx, []SyncNoteInput{input}, false); err != nil || result.Updated != 1 {
		t.Fatalf("reconcile: %+v %v", result, err)
	}
}

func TestSyncSourceBackupAndDeletion(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	input := SyncNoteInput{Path: filepath.Join(t.TempDir(), "note.md"), Title: "note", Body: "body"}
	if _, err := db.SyncNotes(ctx, []SyncNoteInput{input}, true); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM note_sources`).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview source persisted", count, err)
	}
	if _, err := db.SyncNotes(ctx, []SyncNoteInput{input}, false); err != nil {
		t.Fatal(err)
	}
	notes, _ := db.ListAllNotes(ctx)
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	if err := db.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	backup, err := Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if result, err := backup.SyncNotes(ctx, []SyncNoteInput{input}, false); err != nil || result.Unchanged != 1 {
		t.Fatalf("backup lost source: %+v %v", result, err)
	}
	if err := db.DeleteNote(ctx, notes[0].ID); err != nil {
		t.Fatal(err)
	}
	if source, err := db.NoteSource(ctx, notes[0].ID); err != nil || source != "" {
		t.Fatal("source not deleted", source, err)
	}
	if result, err := db.SyncNotes(ctx, []SyncNoteInput{input}, false); err != nil || result.Created != 1 {
		t.Fatalf("recreate: %+v %v", result, err)
	}
}

func TestV2MigrationAddsSourcesWithoutChangingNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range schemaMigrations[:2] {
		if _, err := raw.Exec(migration.SQL); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL); INSERT INTO schema_migrations VALUES(2,'2026-01-01T00:00:00Z'); INSERT INTO notes(title,body,created_at,updated_at) VALUES('old','body','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	notes, err := db.ListAllNotes(context.Background())
	if err != nil || len(notes) != 1 || notes[0].Body != "body" {
		t.Fatalf("migration: %+v %v", notes, err)
	}
	if source, err := db.NoteSource(context.Background(), notes[0].ID); err != nil || source != "" {
		t.Fatal(source, err)
	}
}
