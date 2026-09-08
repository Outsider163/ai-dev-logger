package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const ChunkMaxRunes = 1200

type NoteChunk struct {
	NoteID      int64
	Index       int
	Content     string
	ContentHash string
}

// SplitNoteBody preserves every character, preferring paragraph and line boundaries.
// The hard rune limit also bounds unusually long lines and fenced code blocks.
func SplitNoteBody(body string) []string {
	runes := []rune(body)
	if len(runes) == 0 {
		return []string{""}
	}
	var chunks []string
	for len(runes) > ChunkMaxRunes {
		end := ChunkMaxRunes
		for i := ChunkMaxRunes - 1; i >= ChunkMaxRunes/2; i-- {
			if runes[i] == '\n' && runes[i-1] == '\n' {
				end = i + 1
				break
			}
		}
		if end == ChunkMaxRunes {
			for i := ChunkMaxRunes - 1; i >= ChunkMaxRunes/2; i-- {
				if runes[i] == '\n' {
					end = i + 1
					break
				}
			}
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

func replaceChunks(ctx context.Context, tx *sql.Tx, noteID int64, body string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_chunks WHERE note_id = ?`, noteID); err != nil {
		return err
	}
	for index, content := range SplitNoteBody(body) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_chunks(note_id, chunk_index, content, content_hash) VALUES(?,?,?,?)`, noteID, index, content, hashText(content)); err != nil {
			return err
		}
	}
	return nil
}

func backfillChunks(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,title,body,tags_json,summary,created_at,updated_at FROM notes ORDER BY id`)
	if err != nil {
		return err
	}
	notes, err := scanNotes(rows)
	rows.Close()
	if err != nil {
		return err
	}
	for _, note := range notes {
		if err := replaceChunks(ctx, tx, note.ID, note.Body); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListChunks(ctx context.Context, noteID int64) ([]NoteChunk, error) {
	if _, err := s.GetNote(ctx, noteID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT note_id,chunk_index,content,content_hash FROM note_chunks WHERE note_id=? ORDER BY chunk_index`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []NoteChunk
	for rows.Next() {
		var chunk NoteChunk
		if err := rows.Scan(&chunk.NoteID, &chunk.Index, &chunk.Content, &chunk.ContentHash); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func ChunkEmbeddingText(note Note, content string) string {
	note.Body = content
	return NoteEmbeddingText(note)
}

// EmbeddingsCurrent requires every expected chunk, not just the first vector.
func EmbeddingsCurrent(note Note, embeddings []NoteEmbedding, model string) bool {
	parts := SplitNoteBody(note.Body)
	current := make(map[int]bool)
	for _, e := range embeddings {
		if e.NoteID == note.ID && e.Model == model && e.ChunkIndex >= 0 && e.ChunkIndex < len(parts) {
			current[e.ChunkIndex] = e.MatchesText(ChunkEmbeddingText(note, parts[e.ChunkIndex]))
		}
	}
	for i := range parts {
		if !current[i] {
			return false
		}
	}
	return true
}

// SaveChunkEmbeddings commits a complete snapshot only if the source is unchanged.
func (s *Store) SaveChunkEmbeddings(ctx context.Context, note Note, model string, vectors [][]float64) (NoteEmbedding, error) {
	parts := SplitNoteBody(note.Body)
	if model == "" || len(parts) != len(vectors) {
		return NoteEmbedding{}, fmt.Errorf("invalid chunk embedding batch")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NoteEmbedding{}, err
	}
	defer tx.Rollback()
	current, err := getNote(ctx, tx, note.ID)
	if err != nil {
		return NoteEmbedding{}, err
	}
	if NoteEmbeddingText(current) != NoteEmbeddingText(note) {
		return NoteEmbedding{}, fmt.Errorf("note #%d changed while generating embeddings; retry", note.ID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_embeddings WHERE note_id=? AND model=?`, note.ID, model); err != nil {
		return NoteEmbedding{}, err
	}
	now := time.Now().UTC()
	var first NoteEmbedding
	for i, part := range parts {
		if len(vectors[i]) == 0 || len(vectors[i]) != len(vectors[0]) {
			return NoteEmbedding{}, fmt.Errorf("inconsistent chunk vector dimensions")
		}
		data, err := encodeVector(vectors[i])
		if err != nil {
			return NoteEmbedding{}, err
		}
		hash := hashText(ChunkEmbeddingText(note, part))
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_embeddings(note_id,chunk_index,model,dimensions,vector_json,content_hash,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, note.ID, i, model, len(vectors[i]), data, hash, formatTime(now), formatTime(now)); err != nil {
			return NoteEmbedding{}, err
		}
		if i == 0 {
			first = NoteEmbedding{NoteID: note.ID, Model: model, Dimensions: len(vectors[i]), Vector: vectors[i], ContentHash: hash, CreatedAt: now, UpdatedAt: now}
		}
	}
	if err := tx.Commit(); err != nil {
		return NoteEmbedding{}, err
	}
	return first, nil
}
