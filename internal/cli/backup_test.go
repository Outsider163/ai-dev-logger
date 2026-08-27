package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-dev-logger/internal/store"
)

func TestRunBackupCreatesCompleteDatabaseAndChecksum(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source", "notes.db")
	source, err := store.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	note, err := source.CreateNote(ctx, store.CreateNoteInput{
		Title: "SQLite backup",
		Body:  "VACUUM INTO creates a consistent snapshot.",
		Tags:  []string{"sqlite", "backup"},
	})
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	if _, err := source.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "test-model",
		Text:   store.NoteEmbeddingText(note),
		Vector: []float64{0.4, 0.5},
	}); err != nil {
		source.Close()
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tempDir, "backups", "notes.db")
	var stdout bytes.Buffer
	if err := runBackup(ctx, &stdout, backupOptions{DBPath: sourcePath, Output: outputPath}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"backup created:", outputPath, "size:", "sha256:", "integrity: ok"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected %q in output, got %q", expected, stdout.String())
		}
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(data))
	if !strings.Contains(stdout.String(), "sha256: "+wantHash) {
		t.Fatalf("output checksum does not match backup bytes: %q", stdout.String())
	}

	backup, err := store.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	backedUpNote, err := backup.GetNote(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if backedUpNote.Title != note.Title || backedUpNote.Body != note.Body {
		t.Fatalf("unexpected backed-up note: %#v", backedUpNote)
	}
	if _, err := backup.GetEmbedding(ctx, note.ID, "test-model"); err != nil {
		t.Fatalf("embedding was not backed up: %v", err)
	}
}

func TestRunBackupRefusesExistingOutputWithoutForce(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")
	db, err := store.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tempDir, "backup.db")
	if err := os.WriteFile(outputPath, []byte("original backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = runBackup(ctx, &bytes.Buffer{}, backupOptions{DBPath: sourcePath, Output: outputPath})
	if err == nil || !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("expected overwrite protection error, got %v", err)
	}
	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "original backup" {
		t.Fatalf("existing backup changed without force: %q", data)
	}
}

func TestRunBackupForceReplacesExistingOutput(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "source.db")
	db, err := store.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNote(ctx, store.CreateNoteInput{Title: "replacement", Body: "new data"}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tempDir, "backup.db")
	if err := os.WriteFile(outputPath, []byte("old data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runBackup(ctx, &bytes.Buffer{}, backupOptions{DBPath: sourcePath, Output: outputPath, Force: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDatabase(ctx, outputPath); err != nil {
		t.Fatalf("replacement is not a valid backup: %v", err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected only source and backup files, got %#v", entries)
	}
}

func TestRunBackupRejectsSourceAsOutput(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "notes.db")
	db, err := store.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for _, configuredPath := range []string{sourcePath, sourcePath + "?cache=shared"} {
		err := runBackup(context.Background(), &bytes.Buffer{}, backupOptions{
			DBPath: configuredPath,
			Output: sourcePath,
			Force:  true,
		})
		if err == nil || !strings.Contains(err.Error(), "must not be the source") {
			t.Fatalf("expected same-file protection for %q, got %v", configuredPath, err)
		}
	}
	if err := store.VerifyDatabase(context.Background(), sourcePath); err != nil {
		t.Fatalf("source database was damaged: %v", err)
	}
}

func TestRunBackupRequiresExistingSource(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "missing.db")
	outputPath := filepath.Join(tempDir, "backup.db")
	err := runBackup(context.Background(), &bytes.Buffer{}, backupOptions{
		DBPath: sourcePath,
		Output: outputPath,
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing source error, got %v", err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("missing source should not be created, stat error: %v", err)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("backup should not be created, stat error: %v", err)
	}
}

func TestSHA256File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	data := []byte("ai-dev-logger backup")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	hash, size, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(data))
	if hash != wantHash || size != int64(len(data)) {
		t.Fatalf("expected hash=%s size=%d, got hash=%s size=%d", wantHash, len(data), hash, size)
	}
}
