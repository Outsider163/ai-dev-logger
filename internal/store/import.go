package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type DuplicatePolicy string

const (
	DuplicateSkip  DuplicatePolicy = "skip"
	DuplicateError DuplicatePolicy = "error"
	DuplicateAllow DuplicatePolicy = "allow"
)

type ImportNoteInput struct {
	SourceID  int64
	Title     string
	Body      string
	Tags      []string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ImportNotesOptions struct {
	DuplicatePolicy DuplicatePolicy
	DryRun          bool
}

type ImportNotesResult struct {
	Imported int
	Skipped  int
}

type DuplicateNoteError struct {
	SourceID   int64
	ExistingID int64
	Title      string
}

func (e *DuplicateNoteError) Error() string {
	return fmt.Sprintf(
		"import note #%d %q duplicates existing note #%d",
		e.SourceID,
		e.Title,
		e.ExistingID,
	)
}

// ImportNotes processes the whole batch in one transaction and rolls dry runs back.
func (s *Store) ImportNotes(ctx context.Context, inputs []ImportNoteInput, options ImportNotesOptions) (ImportNotesResult, error) {
	policy := options.DuplicatePolicy
	if policy == "" {
		policy = DuplicateSkip
	}
	if policy != DuplicateSkip && policy != DuplicateError && policy != DuplicateAllow {
		return ImportNotesResult{}, fmt.Errorf("unsupported duplicate policy %q", policy)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportNotesResult{}, err
	}
	defer tx.Rollback()

	duplicateIDs := map[string]int64{}
	if policy != DuplicateAllow {
		rows, err := tx.QueryContext(ctx, `
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
ORDER BY id ASC
`)
		if err != nil {
			return ImportNotesResult{}, err
		}
		existingNotes, scanErr := scanNotes(rows)
		closeErr := rows.Close()
		if scanErr != nil {
			return ImportNotesResult{}, scanErr
		}
		if closeErr != nil {
			return ImportNotesResult{}, closeErr
		}
		for _, note := range existingNotes {
			duplicateIDs[importNoteContentKey(note.Title, note.Body, note.Tags, note.Summary)] = note.ID
		}
	}

	result := ImportNotesResult{}
	for _, input := range inputs {
		if err := validateImportNoteInput(input); err != nil {
			return result, err
		}

		contentKey := importNoteContentKey(input.Title, input.Body, input.Tags, input.Summary)
		if existingID, duplicate := duplicateIDs[contentKey]; duplicate {
			switch policy {
			case DuplicateSkip:
				result.Skipped++
				continue
			case DuplicateError:
				return result, &DuplicateNoteError{
					SourceID:   input.SourceID,
					ExistingID: existingID,
					Title:      input.Title,
				}
			}
		}

		tagsJSON, err := json.Marshal(input.Tags)
		if err != nil {
			return result, fmt.Errorf("encode tags for import note #%d: %w", input.SourceID, err)
		}
		insertResult, err := tx.ExecContext(ctx, `
INSERT INTO notes (title, body, tags_json, summary, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`,
			input.Title,
			input.Body,
			string(tagsJSON),
			input.Summary,
			formatTime(input.CreatedAt),
			formatTime(input.UpdatedAt),
		)
		if err != nil {
			return result, fmt.Errorf("insert import note #%d: %w", input.SourceID, err)
		}
		newID, err := insertResult.LastInsertId()
		if err != nil {
			return result, fmt.Errorf("read imported note #%d id: %w", input.SourceID, err)
		}
		if err := replaceChunks(ctx, tx, newID, input.Body); err != nil {
			return result, err
		}
		result.Imported++
		if policy != DuplicateAllow {
			duplicateIDs[contentKey] = newID
		}
	}

	if options.DryRun {
		return result, nil
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit note import: %w", err)
	}
	return result, nil
}

func validateImportNoteInput(input ImportNoteInput) error {
	if input.SourceID <= 0 {
		return fmt.Errorf("import note source id must be positive, got %d", input.SourceID)
	}
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("import note #%d title is empty", input.SourceID)
	}
	if strings.TrimSpace(input.Body) == "" {
		return fmt.Errorf("import note #%d body is empty", input.SourceID)
	}
	if input.CreatedAt.IsZero() {
		return fmt.Errorf("import note #%d created time is empty", input.SourceID)
	}
	if input.UpdatedAt.IsZero() {
		return fmt.Errorf("import note #%d updated time is empty", input.SourceID)
	}
	if input.UpdatedAt.Before(input.CreatedAt) {
		return fmt.Errorf("import note #%d updated time is before created time", input.SourceID)
	}
	return nil
}

func importNoteContentKey(title string, body string, tags []string, summary string) string {
	// Tag order and letter case do not change a note's content identity.
	canonicalTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		canonicalTags = append(canonicalTags, strings.ToLower(strings.TrimSpace(tag)))
	}
	sort.Strings(canonicalTags)

	data, _ := json.Marshal(struct {
		Title   string   `json:"title"`
		Body    string   `json:"body"`
		Tags    []string `json:"tags"`
		Summary string   `json:"summary"`
	}{
		Title:   title,
		Body:    body,
		Tags:    canonicalTags,
		Summary: summary,
	})
	return string(data)
}
