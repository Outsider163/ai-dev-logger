package store

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSplitNoteBodyPreservesTextAndBounds(t *testing.T) {
	for _, body := range []string{"", "short note", strings.Repeat("中", 1200), strings.Repeat("中🙂", 1400), strings.Repeat("line\r\n", 500), "```go\n" + strings.Repeat("var x = 1\n", 300) + "```", strings.Repeat("a", 800) + "\n\n" + strings.Repeat("b", 800)} {
		parts := SplitNoteBody(body)
		if strings.Join(parts, "") != body {
			t.Fatal("split lost text")
		}
		for _, p := range parts {
			if !utf8.ValidString(p) || len([]rune(p)) > ChunkMaxRunes {
				t.Fatal("invalid or oversized chunk")
			}
		}
	}
	parts := SplitNoteBody(strings.Repeat("a", 800) + "\n\n" + strings.Repeat("b", 800))
	if len(parts) != 2 || !strings.HasSuffix(parts[0], "\n\n") {
		t.Fatalf("paragraph boundary ignored: %d", len(parts))
	}
}

func openChunkTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func assertChunks(t *testing.T, db *Store, note Note) {
	t.Helper()
	chunks, err := db.ListChunks(context.Background(), note.ID)
	if err != nil {
		t.Fatal(err)
	}
	parts := SplitNoteBody(note.Body)
	if len(chunks) != len(parts) {
		t.Fatalf("chunks=%d want=%d", len(chunks), len(parts))
	}
	for i, c := range chunks {
		if c.Index != i || c.Content != parts[i] || c.ContentHash != hashText(c.Content) {
			t.Fatalf("bad chunk %#v", c)
		}
	}
}

func TestChunksFollowCreateUpdateDeleteAndImport(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "long", Body: strings.Repeat("x", 2500)})
	if err != nil {
		t.Fatal(err)
	}
	assertChunks(t, db, note)
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}, {0, 1}, {1, 1}}); err != nil {
		t.Fatal(err)
	}
	status, err := db.GetEmbeddingStatus(ctx, "m")
	if err != nil || status.CurrentForModel != 1 || status.EmbeddingsTotal != 3 {
		t.Fatalf("status %#v: %v", status, err)
	}
	body := "short"
	note, err = db.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	assertChunks(t, db, note)
	vectors, err := db.ListEmbeddings(ctx, "m")
	if err != nil || len(vectors) != 0 {
		t.Fatal("old vectors survived update", err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteNote(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"note_chunks", "note_embeddings"} {
		var count int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphaned %s: %d %v", table, count, err)
		}
	}
	now := time.Now().UTC()
	input := []ImportNoteInput{{SourceID: 9, Title: "imported", Body: strings.Repeat("y", 1600), CreatedAt: now, UpdatedAt: now}}
	if _, err := db.ImportNotes(ctx, input, ImportNotesOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	notes, err := db.ListAllNotes(ctx)
	if err != nil || len(notes) != 0 {
		t.Fatal("dry run saved notes", err)
	}
	if _, err := db.ImportNotes(ctx, input, ImportNotesOptions{}); err != nil {
		t.Fatal(err)
	}
	notes, err = db.ListAllNotes(ctx)
	if err != nil || len(notes) != 1 {
		t.Fatal("import failed", err)
	}
	assertChunks(t, db, notes[0])
}

func TestChunkUpdateFailureRollsBackNoteAndVectors(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "old", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`CREATE TRIGGER reject_chunk BEFORE INSERT ON note_chunks BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("new", 600)
	if _, err := db.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, Body: &body}); err == nil {
		t.Fatal("expected update failure")
	}
	after, err := db.GetNote(ctx, note.ID)
	if err != nil || !reflect.DeepEqual(after, note) {
		t.Fatalf("note changed: %#v %v", after, err)
	}
	assertChunks(t, db, note)
	status, err := db.GetEmbeddingStatus(ctx, "m")
	if err != nil || status.CurrentForModel != 1 {
		t.Fatal("vectors were lost", err)
	}
	if _, err := db.CreateNote(ctx, CreateNoteInput{Title: "failed", Body: "new"}); err == nil {
		t.Fatal("expected create failure")
	}
	notes, _ := db.ListAllNotes(ctx)
	if len(notes) != 1 {
		t.Fatal("partial note saved")
	}
}

func TestChunkEmbeddingBatchRollbackAndConcurrentEdit(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "long", Body: strings.Repeat("x", 1300)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}, {0, 1}}); err != nil {
		t.Fatal(err)
	}
	before, _ := db.ListEmbeddings(ctx, "m")
	for _, vectors := range [][][]float64{{{1}}, {{1, 0}, {}}, {{1, 0}, {1}}, {{1, 0}, {math.NaN(), 0}}} {
		if _, err := db.SaveChunkEmbeddings(ctx, note, "m", vectors); err == nil {
			t.Fatal("expected invalid vector batch failure")
		}
		after, _ := db.ListEmbeddings(ctx, "m")
		if !reflect.DeepEqual(before, after) {
			t.Fatal("failed batch replaced existing vectors")
		}
	}
	title := "changed"
	if _, err := db.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}, {0, 1}}); err == nil {
		t.Fatal("saved outdated snapshot")
	}
	after, _ := db.ListEmbeddings(ctx, "m")
	if len(after) != 0 {
		t.Fatal("stale vectors persisted")
	}
}

func TestV1MigrationPreservesVectorsAndBackfillsChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(schemaMigrations[0].SQL); err != nil {
		t.Fatal(err)
	}
	for i, body := range []string{"short", strings.Repeat("z", 1400)} {
		if _, err := raw.Exec(`INSERT INTO notes(id,title,body,tags_json,summary,created_at,updated_at) VALUES(?, 'legacy',?,'[]','','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, i+1, body); err != nil {
			t.Fatal(err)
		}
		hash := hashText(NoteEmbeddingText(Note{Title: "legacy", Body: body}))
		if _, err := raw.Exec(`INSERT INTO note_embeddings VALUES(?,'m',2,'[1,0]',?,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, i+1, hash); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()
	for attempt := 0; attempt < 2; attempt++ {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		notes, err := db.ListAllNotes(ctx)
		if err != nil || len(notes) != 2 {
			t.Fatal("legacy notes lost", err)
		}
		for _, n := range notes {
			assertChunks(t, db, n)
		}
		status, err := db.GetEmbeddingStatus(ctx, "m")
		if err != nil || status.CurrentForModel != 1 || status.StaleForModel != 1 || status.EmbeddingsTotal != 2 {
			t.Fatalf("migration status %#v %v", status, err)
		}
		db.Close()
	}
}

func TestChunkBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openChunkTestStore(t)
	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "backup", Body: strings.Repeat("b", 1400)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveChunkEmbeddings(ctx, note, "m", [][]float64{{1, 0}, {0, 1}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "backup.db")
	if err := db.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	target := openChunkTestStore(t)
	if err := target.Restore(ctx, path); err != nil {
		t.Fatal(err)
	}
	assertChunks(t, target, note)
	status, err := target.GetEmbeddingStatus(ctx, "m")
	if err != nil || status.CurrentForModel != 1 || status.EmbeddingsTotal != 2 {
		t.Fatalf("restore status %#v %v", status, err)
	}
}
