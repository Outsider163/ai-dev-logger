package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-dev-logger/internal/store"
)

func TestNormalizeImportDuplicatePolicy(t *testing.T) {
	for input, expected := range map[string]store.DuplicatePolicy{
		"":       store.DuplicateSkip,
		"skip":   store.DuplicateSkip,
		" SKIP ": store.DuplicateSkip,
		"error":  store.DuplicateError,
		"ALLOW":  store.DuplicateAllow,
	} {
		actual, err := normalizeImportDuplicatePolicy(input)
		if err != nil {
			t.Fatalf("normalize %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("normalize %q: expected %q, got %q", input, expected, actual)
		}
	}
	if _, err := normalizeImportDuplicatePolicy("replace"); err == nil || !strings.Contains(err.Error(), "skip, error, or allow") {
		t.Fatalf("expected unsupported policy error, got %v", err)
	}
}

func TestReadJSONImportValidatesAndNormalizesNotes(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "notes.json")
	document := validImportDocument()
	document.Notes[0].Title = "  Go mutex  "
	document.Notes[0].Tags = []string{"#Go", "go", " sqlite "}
	document.Notes[0].CreatedAt = "2026-08-01T08:00:00+08:00"
	document.Notes[0].UpdatedAt = "2026-08-01T09:00:00+08:00"
	writeImportDocument(t, path, document)

	inputs, err := readJSONImport(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(inputs))
	}
	input := inputs[0]
	if input.SourceID != 10 || input.Title != "Go mutex" || input.Body != document.Notes[0].Body {
		t.Fatalf("unexpected normalized input: %#v", input)
	}
	if len(input.Tags) != 2 || input.Tags[0] != "Go" || input.Tags[1] != "sqlite" {
		t.Fatalf("unexpected cleaned tags: %#v", input.Tags)
	}
	if input.CreatedAt.Location() != time.UTC || input.CreatedAt.Format(time.RFC3339) != "2026-08-01T00:00:00Z" {
		t.Fatalf("expected UTC created time, got %s", input.CreatedAt)
	}
	if input.UpdatedAt.Format(time.RFC3339) != "2026-08-01T01:00:00Z" {
		t.Fatalf("expected UTC updated time, got %s", input.UpdatedAt)
	}
}

func TestValidateJSONImportRejectsInvalidDocuments(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		mutate   func(*jsonExportDocument)
		expected string
	}{
		{
			name: "schema version",
			mutate: func(document *jsonExportDocument) {
				document.SchemaVersion = 2
			},
			expected: "unsupported import schema_version",
		},
		{
			name: "exported time",
			mutate: func(document *jsonExportDocument) {
				document.ExportedAt = "not-a-time"
			},
			expected: "exported_at is not a valid RFC3339 timestamp",
		},
		{
			name: "notes null",
			mutate: func(document *jsonExportDocument) {
				document.Notes = nil
			},
			expected: "notes must be a JSON array",
		},
		{
			name: "invalid source id",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].ID = 0
			},
			expected: "invalid id",
		},
		{
			name: "duplicate source id",
			mutate: func(document *jsonExportDocument) {
				document.Notes = append(document.Notes, document.Notes[0])
			},
			expected: "duplicate source id",
		},
		{
			name: "empty title",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].Title = " "
			},
			expected: "title is empty",
		},
		{
			name: "empty body",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].Body = "\n"
			},
			expected: "body is empty",
		},
		{
			name: "tags null",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].Tags = nil
			},
			expected: "tags must be a JSON array",
		},
		{
			name: "invalid created time",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].CreatedAt = "yesterday"
			},
			expected: "created_at is not a valid RFC3339 timestamp",
		},
		{
			name: "updated before created",
			mutate: func(document *jsonExportDocument) {
				document.Notes[0].CreatedAt = "2026-08-02T00:00:00Z"
				document.Notes[0].UpdatedAt = "2026-08-01T00:00:00Z"
			},
			expected: "updated_at is before created_at",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			document := validImportDocument()
			testCase.mutate(&document)
			_, err := validateJSONImport(document)
			if err == nil || !strings.Contains(err.Error(), testCase.expected) {
				t.Fatalf("expected error containing %q, got %v", testCase.expected, err)
			}
		})
	}
}

func TestReadJSONImportRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	validDocument := validImportDocument()
	validData, err := json.Marshal(validDocument)
	if err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name: "unknown field",
			data: []byte(`{
  "schema_version": 1,
  "exported_at": "2026-08-01T00:00:00Z",
  "notes": [],
  "unexpected": true
}`),
			expected: "unknown field",
		},
		{
			name:     "trailing JSON value",
			data:     append(append([]byte{}, validData...), []byte("\n{}")...),
			expected: "multiple JSON values",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.json")
			if err := os.WriteFile(path, testCase.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readJSONImport(path)
			if err == nil || !strings.Contains(err.Error(), testCase.expected) {
				t.Fatalf("expected error containing %q, got %v", testCase.expected, err)
			}
		})
	}
}

func TestRunImportDryRunCommitAndDuplicatePolicies(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "notes.json")
	databasePath := filepath.Join(tempDir, "notes.db")
	document := validImportDocument()
	second := document.Notes[0]
	second.ID = 11
	second.Title = "SQLite timeout"
	second.Body = "Set busy_timeout before retrying."
	document.Notes = append(document.Notes, second)
	writeImportDocument(t, inputPath, document)

	var output bytes.Buffer
	err := runImport(ctx, &output, importOptions{
		DBPath: databasePath,
		Input:  inputPath,
		DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "dry run: would import 2 note(s), skip 0 duplicate(s)") {
		t.Fatalf("unexpected dry-run output: %q", output.String())
	}
	assertImportedNoteCount(t, databasePath, 0)

	output.Reset()
	err = runImport(ctx, &output, importOptions{DBPath: databasePath, Input: inputPath})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "imported 2 note(s), skipped 0 duplicate(s)") ||
		!strings.Contains(output.String(), "embed --all") {
		t.Fatalf("unexpected import output: %q", output.String())
	}
	assertImportedNoteCount(t, databasePath, 2)

	output.Reset()
	err = runImport(ctx, &output, importOptions{DBPath: databasePath, Input: inputPath, DuplicatePolicy: "skip"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "imported 0 note(s), skipped 2 duplicate(s)") {
		t.Fatalf("unexpected duplicate skip output: %q", output.String())
	}
	assertImportedNoteCount(t, databasePath, 2)

	output.Reset()
	err = runImport(ctx, &output, importOptions{DBPath: databasePath, Input: inputPath, DuplicatePolicy: "allow"})
	if err != nil {
		t.Fatal(err)
	}
	assertImportedNoteCount(t, databasePath, 4)

	err = runImport(ctx, &bytes.Buffer{}, importOptions{DBPath: databasePath, Input: inputPath, DuplicatePolicy: "error"})
	var duplicateErr *store.DuplicateNoteError
	if !errors.As(err, &duplicateErr) {
		t.Fatalf("expected DuplicateNoteError, got %v", err)
	}
	assertImportedNoteCount(t, databasePath, 4)
}

func TestRunImportRejectsInvalidFileBeforeOpeningDatabase(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "invalid.json")
	databasePath := filepath.Join(tempDir, "new", "notes.db")
	document := validImportDocument()
	document.SchemaVersion = 999
	writeImportDocument(t, inputPath, document)

	err := runImport(context.Background(), &bytes.Buffer{}, importOptions{
		DBPath: databasePath,
		Input:  inputPath,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported import schema_version") {
		t.Fatalf("expected schema error, got %v", err)
	}
	if _, statErr := os.Stat(databasePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid import should not create database, stat error: %v", statErr)
	}
}

func validImportDocument() jsonExportDocument {
	return jsonExportDocument{
		SchemaVersion: exportSchemaVersion,
		ExportedAt:    "2026-08-01T02:00:00Z",
		Notes: []jsonExportNote{
			{
				ID:        10,
				Title:     "Go mutex",
				Body:      "Use sync.Mutex to protect shared state.",
				Tags:      []string{"go", "concurrency"},
				Summary:   "Protect shared state",
				CreatedAt: "2026-08-01T00:00:00Z",
				UpdatedAt: "2026-08-01T01:00:00Z",
			},
		},
	}
}

func writeImportDocument(t *testing.T, path string, document jsonExportDocument) {
	t.Helper()
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertImportedNoteCount(t *testing.T, databasePath string, expected int) {
	t.Helper()
	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	notes, err := db.ListAllNotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != expected {
		t.Fatalf("expected %d note(s), got %d", expected, len(notes))
	}
}
