package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-dev-logger/internal/store"
)

func TestNormalizeExportFormat(t *testing.T) {
	for input, expected := range map[string]string{
		"markdown":   "markdown",
		" Markdown ": "markdown",
		"md":         "markdown",
		"JSON":       "json",
	} {
		actual, err := normalizeExportFormat(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalize %q: expected %q, got %q", input, expected, actual)
		}
	}

	if _, err := normalizeExportFormat("csv"); err == nil || !strings.Contains(err.Error(), "markdown or json") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestRenderJSONExportPreservesFields(t *testing.T) {
	createdAt := time.Date(2026, time.August, 1, 2, 3, 4, 500, time.FixedZone("UTC+8", 8*60*60))
	updatedAt := createdAt.Add(time.Hour)
	exportedAt := updatedAt.Add(time.Hour)
	notes := []store.Note{
		{
			ID:        7,
			Title:     "Go mutex",
			Body:      "Use a lock.\n\n```go\nvar mu sync.Mutex\n```",
			Tags:      []string{"go", "concurrency"},
			Summary:   "保护共享状态",
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		{
			ID:        8,
			Title:     "No tags",
			Body:      "body",
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
	}

	data, err := renderJSONExport(notes, exportedAt)
	if err != nil {
		t.Fatal(err)
	}
	if data[len(data)-1] != '\n' {
		t.Fatal("expected JSON export to end with a newline")
	}

	var document jsonExportDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != exportSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", exportSchemaVersion, document.SchemaVersion)
	}
	if document.ExportedAt != formatExportTime(exportedAt) {
		t.Fatalf("unexpected exported_at: %q", document.ExportedAt)
	}
	if len(document.Notes) != 2 {
		t.Fatalf("expected two notes, got %d", len(document.Notes))
	}
	first := document.Notes[0]
	if first.ID != 7 || first.Title != notes[0].Title || first.Body != notes[0].Body || first.Summary != notes[0].Summary {
		t.Fatalf("first note fields changed: %#v", first)
	}
	if len(first.Tags) != 2 || first.Tags[1] != "concurrency" {
		t.Fatalf("unexpected tags: %#v", first.Tags)
	}
	if first.CreatedAt != formatExportTime(createdAt) || first.UpdatedAt != formatExportTime(updatedAt) {
		t.Fatalf("unexpected note times: %#v", first)
	}
	if document.Notes[1].Tags == nil || len(document.Notes[1].Tags) != 0 {
		t.Fatalf("expected empty tags to be [], got %#v", document.Notes[1].Tags)
	}
}

func TestRenderMarkdownExportKeepsReadableStructure(t *testing.T) {
	now := time.Date(2026, time.August, 2, 3, 4, 5, 0, time.UTC)
	note := store.Note{
		ID:        3,
		Title:     "Go map\nconcurrency",
		Body:      "Problem:\n\n```go\nfatal error: concurrent map writes\n```",
		Tags:      []string{"go", "map`issue"},
		Summary:   "第一行\n第二行",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Minute),
	}

	output := string(renderMarkdownExport([]store.Note{note}, now.Add(time.Hour)))
	for _, expected := range []string{
		"# ai-dev-logger notes",
		"- Notes: `1`",
		"## Note #3: Go map concurrency",
		"- Tags: `go`, `map'issue`",
		"> 第一行\n> 第二行",
		"### Content",
		note.Body,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in Markdown export, got:\n%s", expected, output)
		}
	}
}

func TestRenderEmptyExports(t *testing.T) {
	now := time.Date(2026, time.August, 3, 4, 5, 6, 0, time.UTC)
	markdown := string(renderMarkdownExport(nil, now))
	if !strings.Contains(markdown, "- Notes: `0`") || !strings.Contains(markdown, "No notes found.") {
		t.Fatalf("unexpected empty Markdown export: %s", markdown)
	}

	data, err := renderJSONExport(nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"notes": []`) {
		t.Fatalf("expected empty JSON notes array, got %s", data)
	}
}

func TestRunExportWritesAllNotes(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "data", "notes.db")
	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []store.CreateNoteInput{
		{Title: "First", Body: "first body", Tags: []string{"go"}},
		{Title: "Second", Body: "second body", Summary: "second summary"},
	} {
		if _, err := db.CreateNote(ctx, input); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tempDir, "exports", "notes.json")
	var stdout bytes.Buffer
	err = runExport(ctx, &stdout, exportOptions{
		DBPath:     databasePath,
		Format:     "json",
		Output:     outputPath,
		ExportedAt: time.Date(2026, time.August, 4, 5, 6, 7, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "exported 2 note(s)") || !strings.Contains(stdout.String(), "(json)") {
		t.Fatalf("unexpected command output: %q", stdout.String())
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var document jsonExportDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Notes) != 2 || document.Notes[0].Title != "First" || document.Notes[1].Title != "Second" {
		t.Fatalf("unexpected exported notes: %#v", document.Notes)
	}
}

func TestWriteExportFileRequiresForceToOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := writeExportFile(path, []byte("replacement"), false)
	if err == nil || !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("expected overwrite protection error, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("existing file changed without force: %q", data)
	}

	if err := writeExportFile(path, []byte("replacement"), true); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "replacement" {
		t.Fatalf("expected force overwrite, got %q", data)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("expected no temporary files after replacement, got %#v", entries)
	}
}

func TestRunExportRejectsDatabaseAsOutput(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "notes.db")
	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for _, configuredPath := range []string{databasePath, databasePath + "?cache=shared"} {
		err = runExport(context.Background(), &bytes.Buffer{}, exportOptions{
			DBPath: configuredPath,
			Format: "json",
			Output: databasePath,
			Force:  true,
		})
		if err == nil || !strings.Contains(err.Error(), "must not be the SQLite database") {
			t.Fatalf("expected database path protection error for %q, got %v", configuredPath, err)
		}
	}

	db, err = store.Open(databasePath)
	if err != nil {
		t.Fatalf("database was damaged by rejected export: %v", err)
	}
	defer db.Close()
	if _, err := db.ListAllNotes(context.Background()); err != nil {
		t.Fatalf("database was damaged by rejected export: %v", err)
	}
}

func TestPathsReferToSameFileResolvesParentSymlink(t *testing.T) {
	tempDir := t.TempDir()
	realDirectory := filepath.Join(tempDir, "real")
	if err := os.Mkdir(realDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedDirectory := filepath.Join(tempDir, "linked")
	if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	if !pathsReferToSameFile(
		filepath.Join(realDirectory, "future.db"),
		filepath.Join(linkedDirectory, "future.db"),
	) {
		t.Fatal("expected paths through a parent symlink to compare as the same future file")
	}
}
