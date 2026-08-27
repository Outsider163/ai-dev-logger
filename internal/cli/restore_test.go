package cli

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-dev-logger/internal/store"
)

func TestRunRestoreDryRunDoesNotChangeTarget(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "restore-input.db")
	createRestoreTestDatabase(t, inputPath, []string{"source first", "source second"}, true)
	targetPath := filepath.Join(tempDir, "target.db")
	createRestoreTestDatabase(t, targetPath, []string{"keep current"}, false)

	targetHashBefore, _, err := sha256File(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 12, 34, 56, 0, time.UTC)
	safetyPath, err := nextRestoreSafetyPath(targetPath, now)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runRestore(ctx, &stdout, restoreOptions{
		DBPath: targetPath,
		Input:  inputPath,
		DryRun: true,
		Now:    now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"restore source verified:",
		"schema version: 1",
		"notes: 2",
		"embeddings: 1",
		"target notes: 1",
		"safety backup: would create " + safetyPath,
		"dry run: no files changed",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected %q in dry-run output, got %q", expected, stdout.String())
		}
	}

	targetHashAfter, _, err := sha256File(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if targetHashAfter != targetHashBefore {
		t.Fatal("dry run changed the target database")
	}
	if _, err := os.Stat(safetyPath); !os.IsNotExist(err) {
		t.Fatalf("dry run created a safety backup, stat error: %v", err)
	}
}

func TestRunRestoreRequiresExplicitYes(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "restore-input.db")
	createRestoreTestDatabase(t, inputPath, []string{"new source"}, false)
	targetPath := filepath.Join(tempDir, "target.db")
	createRestoreTestDatabase(t, targetPath, []string{"keep current"}, false)
	now := time.Date(2026, time.August, 27, 1, 2, 3, 0, time.UTC)
	safetyPath, err := nextRestoreSafetyPath(targetPath, now)
	if err != nil {
		t.Fatal(err)
	}

	err = runRestore(context.Background(), &bytes.Buffer{}, restoreOptions{
		DBPath: targetPath,
		Input:  inputPath,
		Now:    now,
	})
	if err == nil || !strings.Contains(err.Error(), "rerun with --yes") {
		t.Fatalf("expected explicit confirmation error, got %v", err)
	}
	if _, err := os.Stat(safetyPath); !os.IsNotExist(err) {
		t.Fatalf("unconfirmed restore created a safety backup, stat error: %v", err)
	}
	assertRestoreTestTitles(t, targetPath, []string{"keep current"})
}

func TestRunRestoreCreatesSafetyBackupAndRestoresDatabase(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "restore-input.db")
	createRestoreTestDatabase(t, inputPath, []string{"restored first", "restored second"}, true)
	targetPath := filepath.Join(tempDir, "target.db")
	createRestoreTestDatabase(t, targetPath, []string{"old current note"}, false)
	inputHashBefore, _, err := sha256File(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 27, 4, 5, 6, 0, time.UTC)
	safetyPath, err := nextRestoreSafetyPath(targetPath, now)
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := runRestore(ctx, &stdout, restoreOptions{
		DBPath: targetPath,
		Input:  inputPath,
		Yes:    true,
		Now:    now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"restore source verified:",
		"notes: 2",
		"embeddings: 1",
		"safety backup created: " + safetyPath,
		"safety backup sha256:",
		"restored database: " + targetPath,
		"integrity: ok",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected %q in restore output, got %q", expected, stdout.String())
		}
	}

	assertRestoreTestTitles(t, targetPath, []string{"restored first", "restored second"})
	targetInfo, err := store.InspectDatabase(ctx, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if targetInfo.Embeddings != 1 {
		t.Fatalf("expected restored embedding, got %#v", targetInfo)
	}
	assertRestoreTestTitles(t, safetyPath, []string{"old current note"})
	safetyHash, _, err := sha256File(safetyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "safety backup sha256: "+safetyHash) {
		t.Fatalf("safety checksum does not match output: %q", stdout.String())
	}
	inputHashAfter, _, err := sha256File(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if inputHashAfter != inputHashBefore {
		t.Fatal("restore changed the input backup")
	}
}

func TestRunRestoreCreatesMissingTargetWithoutSafetyBackup(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "restore-input.db")
	createRestoreTestDatabase(t, inputPath, []string{"only source"}, false)
	targetPath := filepath.Join(tempDir, "new", "target.db")

	var stdout bytes.Buffer
	if err := runRestore(context.Background(), &stdout, restoreOptions{
		DBPath: targetPath,
		Input:  inputPath,
		Yes:    true,
		Now:    time.Date(2026, time.August, 27, 7, 8, 9, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "safety backup created:") {
		t.Fatalf("unexpected safety backup for missing target: %q", stdout.String())
	}
	assertRestoreTestTitles(t, targetPath, []string{"only source"})
	entries, err := os.ReadDir(filepath.Dir(targetPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(targetPath) {
		t.Fatalf("unexpected files beside new target: %#v", entries)
	}
}

func TestRunRestoreRejectsSameFileAndNonProjectInput(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "target.db")
	createRestoreTestDatabase(t, targetPath, []string{"current"}, false)

	err := runRestore(context.Background(), &bytes.Buffer{}, restoreOptions{
		DBPath: targetPath,
		Input:  targetPath,
		DryRun: true,
	})
	if err == nil || !strings.Contains(err.Error(), "must not be the target") {
		t.Fatalf("expected same-file protection error, got %v", err)
	}

	nonProjectPath := filepath.Join(tempDir, "other.db")
	nonProject, err := sql.Open("sqlite", nonProjectPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nonProject.Exec(`CREATE TABLE example (id INTEGER PRIMARY KEY)`); err != nil {
		nonProject.Close()
		t.Fatal(err)
	}
	if err := nonProject.Close(); err != nil {
		t.Fatal(err)
	}
	err = runRestore(context.Background(), &bytes.Buffer{}, restoreOptions{
		DBPath: targetPath,
		Input:  nonProjectPath,
		DryRun: true,
	})
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected non-project input error, got %v", err)
	}
	assertRestoreTestTitles(t, targetPath, []string{"current"})
}

func TestNextRestoreSafetyPathAvoidsCollisions(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "notes.db")
	now := time.Date(2026, time.August, 27, 10, 11, 12, 0, time.UTC)
	first, err := nextRestoreSafetyPath(targetPath, now)
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := filepath.Join(tempDir, "notes.pre-restore-20260827T101112Z.db")
	if first != wantFirst {
		t.Fatalf("expected %q, got %q", wantFirst, first)
	}
	if err := os.WriteFile(first, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := nextRestoreSafetyPath(targetPath, now)
	if err != nil {
		t.Fatal(err)
	}
	wantSecond := filepath.Join(tempDir, "notes.pre-restore-20260827T101112Z-1.db")
	if second != wantSecond {
		t.Fatalf("expected %q, got %q", wantSecond, second)
	}
}

func TestVerifyRestoreInputUnchangedDetectsModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restore-input.db")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, size, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyRestoreInputUnchanged(path, hash, size); err != nil {
		t.Fatalf("unchanged input should pass: %v", err)
	}
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRestoreInputUnchanged(path, hash, size); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected changed input error, got %v", err)
	}
}

func createRestoreTestDatabase(t *testing.T, path string, titles []string, withEmbedding bool) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for index, title := range titles {
		note, err := db.CreateNote(ctx, store.CreateNoteInput{
			Title: title,
			Body:  "body for " + title,
			Tags:  []string{"restore-test"},
		})
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if withEmbedding && index == 0 {
			if _, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{
				NoteID: note.ID,
				Model:  "restore-test-model",
				Text:   store.NoteEmbeddingText(note),
				Vector: []float64{0.2, 0.4, 0.6},
			}); err != nil {
				db.Close()
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertRestoreTestTitles(t *testing.T, path string, expected []string) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	notes, err := db.ListAllNotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != len(expected) {
		t.Fatalf("expected %d note(s), got %#v", len(expected), notes)
	}
	for index, title := range expected {
		if notes[index].Title != title {
			t.Fatalf("expected title %q at index %d, got %#v", title, index, notes)
		}
	}
}
