package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type SyncNoteInput struct {
	Path  string
	Title string
	Body  string
	Tags  []string
}

type SyncNotesResult struct {
	Created   int
	Updated   int
	Unchanged int
}

// SyncNotes commits source tracking, text, chunks and vector invalidation together.
func (s *Store) SyncNotes(ctx context.Context, inputs []SyncNoteInput, dryRun bool) (SyncNotesResult, error) {
	result := SyncNotesResult{}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	for _, input := range inputs {
		if !filepath.IsAbs(input.Path) || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" {
			return result, fmt.Errorf("sync requires an absolute source path, title and body")
		}
		path := filepath.Clean(input.Path)
		if runtime.GOOS == "windows" {
			path = strings.ToLower(path)
		}
		var id int64
		var previousHash string
		err := tx.QueryRowContext(ctx, `SELECT note_id, body_hash FROM note_sources WHERE path=?`, path).Scan(&id, &previousHash)
		bodyHash := hashText(input.Body)
		now := formatTime(time.Now().UTC())
		switch {
		case errors.Is(err, sql.ErrNoRows):
			tags, err := json.Marshal(input.Tags)
			if err != nil {
				return result, err
			}
			insert, err := tx.ExecContext(ctx, `INSERT INTO notes(title,body,tags_json,summary,created_at,updated_at) VALUES(?,?,?,'',?,?)`, input.Title, input.Body, string(tags), now, now)
			if err != nil {
				return result, err
			}
			id, err = insert.LastInsertId()
			if err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_sources(path,note_id,body_hash) VALUES(?,?,?)`, path, id, bodyHash); err != nil {
				return result, err
			}
			result.Created++
		case err != nil:
			return result, err
		default:
			if bodyHash == previousHash {
				result.Unchanged++
				continue
			}
			note, err := getNote(ctx, tx, id)
			if err != nil {
				return result, err
			}
			if hashText(note.Body) != previousHash && note.Body != input.Body {
				return result, fmt.Errorf("sync conflict for %q: note #%d was edited locally; reconcile its body with the source file before retrying", input.Path, id)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE notes SET body=?,summary='',updated_at=? WHERE id=?`, input.Body, now, id); err != nil {
				return result, err
			}
			if err := deleteEmbeddings(ctx, tx, id); err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE note_sources SET body_hash=? WHERE path=?`, bodyHash, path); err != nil {
				return result, err
			}
			result.Updated++
		}
		if err := replaceChunks(ctx, tx, id, input.Body); err != nil {
			return result, err
		}
	}
	if dryRun {
		return result, nil
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) NoteSource(ctx context.Context, id int64) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT path FROM note_sources WHERE note_id=?`, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return path, err
}
