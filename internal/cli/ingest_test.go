package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-dev-logger/internal/store"
)

func TestIngestPreservesContentChunksAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.md")
	body := "# 学习资料\r\n" + strings.Repeat("并发访问需要同步。\n", 200)
	if err := os.WriteFile(path, append([]byte{0xef, 0xbb, 0xbf}, []byte(body)...), 0600); err != nil {
		t.Fatal(err)
	}
	options := ingestOptions{DBPath: filepath.Join(dir, "notes.db"), Paths: []string{path}, Tags: []string{"#go", "go"}, DryRun: true}
	var output bytes.Buffer
	if err := runIngest(context.Background(), &output, options); err != nil {
		t.Fatal(err)
	}
	assertImportedNoteCount(t, options.DBPath, 0)
	options.DryRun = false
	for i := 0; i < 2; i++ {
		if err := runIngest(context.Background(), &output, options); err != nil {
			t.Fatal(err)
		}
	}
	assertImportedNoteCount(t, options.DBPath, 1)
	db, err := store.Open(options.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	notes, err := db.ListNotes(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if notes[0].Title != "notes.md" || notes[0].Body != body || len(notes[0].Tags) != 1 {
		t.Fatalf("unexpected imported note: %#v", notes[0])
	}
	chunks, err := db.ListChunks(context.Background(), notes[0].ID)
	if err != nil || len(chunks) < 2 {
		t.Fatalf("expected automatic chunks, got %d: %v", len(chunks), err)
	}
	if !strings.Contains(output.String(), "skipped 1 duplicate(s)") {
		t.Fatal(output.String())
	}
}

func TestIngestRejectsBadBatchBeforeOpeningDatabase(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{"empty", []byte(" \n")}, {"binary", []byte("abc\x00def")},
		{"encoding", []byte{0xff, 0xfe}}, {"large", bytes.Repeat([]byte("x"), maxIngestFileBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			good, bad := filepath.Join(dir, "main.go"), filepath.Join(dir, "bad.txt")
			if err := os.WriteFile(good, []byte("package main\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bad, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			dbPath := filepath.Join(dir, "notes.db")
			err := runIngest(context.Background(), &bytes.Buffer{}, ingestOptions{DBPath: dbPath, Paths: []string{good, bad}})
			if err == nil {
				t.Fatal("expected validation failure")
			}
			if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
				t.Fatalf("database created for invalid batch: %v", err)
			}
		})
	}
}

func TestIngestDuplicateErrorRollsBackWholeBatch(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	options := ingestOptions{DBPath: filepath.Join(dir, "notes.db"), Paths: []string{first}}
	if err := runIngest(context.Background(), &bytes.Buffer{}, options); err != nil {
		t.Fatal(err)
	}
	options.Paths, options.DuplicatePolicy = []string{second, first}, "error"
	if err := runIngest(context.Background(), &bytes.Buffer{}, options); err == nil {
		t.Fatal("expected duplicate error")
	}
	assertImportedNoteCount(t, options.DBPath, 1)
}
