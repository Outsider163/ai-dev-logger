package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const sqliteBusyTimeoutMilliseconds = 5000

var ErrNoteNotFound = errors.New("note not found")

type Note struct {
	ID        int64
	Title     string
	Body      string
	Tags      []string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CreateNoteInput struct {
	Title   string
	Body    string
	Tags    []string
	Summary string
}

type UpdateNoteInput struct {
	ID          int64
	Title       *string
	Body        *string
	Tags        []string
	ReplaceTags bool
	Summary     *string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func sqliteDSN(path string) string {
	params := url.Values{}
	params.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMilliseconds))
	params.Add("_pragma", "foreign_keys(ON)")

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + params.Encode()
}

func (s *Store) CreateNote(ctx context.Context, input CreateNoteInput) (Note, error) {
	now := time.Now().UTC()
	tagsJSON, err := json.Marshal(input.Tags)
	if err != nil {
		return Note{}, err
	}

	result, err := s.db.ExecContext(ctx, `
INSERT INTO notes (title, body, tags_json, summary, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
`, input.Title, input.Body, string(tagsJSON), input.Summary, formatTime(now), formatTime(now))
	if err != nil {
		return Note{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Note{}, err
	}

	return Note{
		ID:        id,
		Title:     input.Title,
		Body:      input.Body,
		Tags:      input.Tags,
		Summary:   input.Summary,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *Store) ListNotes(ctx context.Context, limit int) ([]Note, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
ORDER BY created_at DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

// ListAllNotes returns every note for maintenance tasks such as re-embedding.
func (s *Store) ListAllNotes(ctx context.Context) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

func (s *Store) GetNote(ctx context.Context, id int64) (Note, error) {
	return getNote(ctx, s.db, id)
}

type noteQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getNote(ctx context.Context, querier noteQuerier, id int64) (Note, error) {
	row := querier.QueryRowContext(ctx, `
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
WHERE id = ?
`, id)

	note, err := scanNote(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNoteNotFound
	}
	if err != nil {
		return Note{}, err
	}

	return note, nil
}

func (s *Store) UpdateNote(ctx context.Context, input UpdateNoteInput) (Note, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Note{}, err
	}
	defer tx.Rollback()

	note, err := getNote(ctx, tx, input.ID)
	if err != nil {
		return Note{}, err
	}

	if input.Title != nil {
		note.Title = *input.Title
	}
	if input.Body != nil {
		note.Body = *input.Body
	}
	if input.ReplaceTags {
		note.Tags = input.Tags
	}
	if input.Summary != nil {
		note.Summary = *input.Summary
	}
	note.UpdatedAt = time.Now().UTC()

	tagsJSON, err := json.Marshal(note.Tags)
	if err != nil {
		return Note{}, err
	}

	result, err := tx.ExecContext(ctx, `
UPDATE notes
SET title = ?, body = ?, tags_json = ?, summary = ?, updated_at = ?
WHERE id = ?
`, note.Title, note.Body, string(tagsJSON), note.Summary, formatTime(note.UpdatedAt), note.ID)
	if err != nil {
		return Note{}, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return Note{}, err
	}
	if affected == 0 {
		return Note{}, ErrNoteNotFound
	}

	// The note text changed, so all of its stored vectors are stale.
	if err := deleteEmbeddings(ctx, tx, note.ID); err != nil {
		return Note{}, err
	}
	if err := tx.Commit(); err != nil {
		return Note{}, err
	}

	return note, nil
}

func (s *Store) DeleteNote(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM notes
WHERE id = ?
`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNoteNotFound
	}

	// SQLite removes related embeddings in the same statement through ON DELETE CASCADE.
	return nil
}

func (s *Store) SearchNotes(ctx context.Context, query string, limit int) ([]Note, error) {
	if limit <= 0 {
		limit = 10
	}

	likeQuery := "%" + escapeLike(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, body, tags_json, summary, created_at, updated_at
FROM notes
WHERE title LIKE ? ESCAPE '\'
	OR body LIKE ? ESCAPE '\'
	OR tags_json LIKE ? ESCAPE '\'
ORDER BY created_at DESC
LIMIT ?
`, likeQuery, likeQuery, likeQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNotes(rows)
}

type noteScanner interface {
	Scan(dest ...any) error
}

func scanNote(scanner noteScanner) (Note, error) {
	var note Note
	var tagsJSON string
	var createdAt string
	var updatedAt string

	if err := scanner.Scan(
		&note.ID,
		&note.Title,
		&note.Body,
		&tagsJSON,
		&note.Summary,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Note{}, err
	}

	if err := json.Unmarshal([]byte(tagsJSON), &note.Tags); err != nil {
		return Note{}, fmt.Errorf("decode tags for note %d: %w", note.ID, err)
	}

	var err error
	note.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Note{}, err
	}
	note.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Note{}, err
	}

	return note, nil
}

func scanNotes(rows *sql.Rows) ([]Note, error) {
	var notes []Note
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}

		notes = append(notes, note)
	}

	return notes, rows.Err()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
