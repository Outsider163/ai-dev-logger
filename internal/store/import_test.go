package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestImportNotesCommitsAndPreservesContent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	createdAt := time.Date(2026, time.August, 1, 2, 3, 4, 5, time.UTC)
	inputs := []ImportNoteInput{
		{
			SourceID:  41,
			Title:     "First",
			Body:      "first body",
			Tags:      []string{"go", "sqlite"},
			Summary:   "first summary",
			CreatedAt: createdAt,
			UpdatedAt: createdAt.Add(time.Minute),
		},
		{
			SourceID:  99,
			Title:     "Second",
			Body:      "second body",
			Tags:      []string{},
			CreatedAt: createdAt.Add(time.Hour),
			UpdatedAt: createdAt.Add(time.Hour),
		},
	}

	result, err := db.ImportNotes(ctx, inputs, ImportNotesOptions{DuplicatePolicy: DuplicateSkip})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Skipped != 0 {
		t.Fatalf("unexpected import result: %#v", result)
	}

	notes, err := db.ListAllNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected two notes, got %d", len(notes))
	}
	if notes[0].ID == inputs[0].SourceID {
		t.Fatal("source IDs should not replace local SQLite IDs")
	}
	if notes[0].Title != inputs[0].Title || notes[0].Body != inputs[0].Body || notes[0].Summary != inputs[0].Summary {
		t.Fatalf("imported content changed: %#v", notes[0])
	}
	if len(notes[0].Tags) != 2 || notes[0].Tags[1] != "sqlite" {
		t.Fatalf("unexpected imported tags: %#v", notes[0].Tags)
	}
	if !notes[0].CreatedAt.Equal(inputs[0].CreatedAt) || !notes[0].UpdatedAt.Equal(inputs[0].UpdatedAt) {
		t.Fatalf("imported times changed: %#v", notes[0])
	}
}

func TestImportNotesDryRunRollsBack(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	result, err := db.ImportNotes(ctx, []ImportNoteInput{testImportNote(1, "Dry run")}, ImportNotesOptions{
		DuplicatePolicy: DuplicateSkip,
		DryRun:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}

	notes, err := db.ListAllNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Fatalf("dry-run persisted %d note(s)", len(notes))
	}
}

func TestImportNotesDuplicatePolicies(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		policy           DuplicatePolicy
		expectedImported int
		expectedSkipped  int
		expectedCount    int
		expectError      bool
	}{
		{name: "skip", policy: DuplicateSkip, expectedSkipped: 1, expectedCount: 1},
		{name: "error", policy: DuplicateError, expectedCount: 1, expectError: true},
		{name: "allow", policy: DuplicateAllow, expectedImported: 1, expectedCount: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			existing, err := db.CreateNote(ctx, CreateNoteInput{
				Title:   "Same",
				Body:    "same body",
				Tags:    []string{"Go", "SQLite"},
				Summary: "same summary",
			})
			if err != nil {
				t.Fatal(err)
			}
			input := testImportNote(77, "Same")
			input.Body = "same body"
			input.Tags = []string{"sqlite", "go"}
			input.Summary = "same summary"

			result, importErr := db.ImportNotes(ctx, []ImportNoteInput{input}, ImportNotesOptions{
				DuplicatePolicy: testCase.policy,
			})
			if testCase.expectError {
				var duplicateErr *DuplicateNoteError
				if !errors.As(importErr, &duplicateErr) {
					t.Fatalf("expected DuplicateNoteError, got %v", importErr)
				}
				if duplicateErr.SourceID != input.SourceID || duplicateErr.ExistingID != existing.ID {
					t.Fatalf("unexpected duplicate error: %#v", duplicateErr)
				}
			} else if importErr != nil {
				t.Fatal(importErr)
			}
			if result.Imported != testCase.expectedImported || result.Skipped != testCase.expectedSkipped {
				t.Fatalf("unexpected result: %#v", result)
			}

			notes, err := db.ListAllNotes(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(notes) != testCase.expectedCount {
				t.Fatalf("expected %d note(s), got %d", testCase.expectedCount, len(notes))
			}
		})
	}
}

func TestImportNotesDuplicateErrorRollsBackEarlierInserts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	existingInput := testImportNote(1, "Existing")
	if _, err := db.ImportNotes(ctx, []ImportNoteInput{existingInput}, ImportNotesOptions{}); err != nil {
		t.Fatal(err)
	}

	uniqueInput := testImportNote(2, "Must roll back")
	duplicateInput := testImportNote(3, "Existing")
	result, err := db.ImportNotes(ctx, []ImportNoteInput{uniqueInput, duplicateInput}, ImportNotesOptions{
		DuplicatePolicy: DuplicateError,
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if result.Imported != 1 {
		t.Fatalf("expected one attempted insert before the duplicate, got %#v", result)
	}

	notes, err := db.ListAllNotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Title != "Existing" {
		t.Fatalf("transaction did not roll back: %#v", notes)
	}
}

func TestImportNotesSkipsDuplicateWithinBatch(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "notes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	first := testImportNote(1, "Batch duplicate")
	first.Tags = []string{"go", "sqlite"}
	second := testImportNote(2, "Batch duplicate")
	second.Tags = []string{"SQLITE", "GO"}

	result, err := db.ImportNotes(ctx, []ImportNoteInput{first, second}, ImportNotesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected batch duplicate result: %#v", result)
	}
}

func testImportNote(sourceID int64, title string) ImportNoteInput {
	createdAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	return ImportNoteInput{
		SourceID:  sourceID,
		Title:     title,
		Body:      title + " body",
		Tags:      []string{},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
}
