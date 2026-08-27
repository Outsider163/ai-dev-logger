package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupCreatesCompleteVerifiedSnapshot(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source", "notes.db")
	db, err := Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{
		Title:   "Go mutex",
		Body:    "Use sync.Mutex to protect shared state.",
		Tags:    []string{"go", "concurrency"},
		Summary: "Protect shared state.",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := NoteEmbeddingText(note)
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "test-embedding-model",
		Text:   text,
		Vector: []float64{0.1, 0.2, 0.3},
	}); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(tempDir, "backups", "notes-backup.db")
	if err := db.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDatabase(ctx, backupPath); err != nil {
		t.Fatalf("backup should pass integrity check: %v", err)
	}

	backup, err := Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()

	notes, err := backup.ListAllNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Title != note.Title || notes[0].Body != note.Body {
		t.Fatalf("backup did not preserve note: %#v", notes)
	}
	embedding, err := backup.GetEmbedding(ctx, note.ID, "test-embedding-model")
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding.Vector) != 3 || embedding.Vector[2] != 0.3 || !embedding.MatchesText(text) {
		t.Fatalf("backup did not preserve embedding: %#v", embedding)
	}
}

func TestBackupRefusesExistingDestination(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	db, err := Open(filepath.Join(tempDir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	destination := filepath.Join(tempDir, "existing.db")
	if err := os.WriteFile(destination, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = db.Backup(ctx, destination)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing destination error, got %v", err)
	}
	data, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "keep me" {
		t.Fatalf("existing destination changed: %q", data)
	}
}

func TestVerifyDatabaseRejectsInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(path, []byte("this is not SQLite"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := VerifyDatabase(context.Background(), path)
	if err == nil || !strings.Contains(err.Error(), "integrity check") {
		t.Fatalf("expected integrity check error, got %v", err)
	}
}

func TestVerifyDatabaseRejectsMissingFileAndDirectory(t *testing.T) {
	tempDir := t.TempDir()
	if err := VerifyDatabase(context.Background(), filepath.Join(tempDir, "missing.db")); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
	if err := VerifyDatabase(context.Background(), tempDir); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("expected directory error, got %v", err)
	}
}
