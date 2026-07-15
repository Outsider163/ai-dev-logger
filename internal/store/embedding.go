package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrEmbeddingNotFound = errors.New("embedding not found")

type NoteEmbedding struct {
	NoteID      int64
	Model       string
	Dimensions  int
	Vector      []float64
	ContentHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpsertEmbeddingInput struct {
	NoteID int64
	Model  string
	Text   string
	Vector []float64
}

func (s *Store) UpsertEmbedding(ctx context.Context, input UpsertEmbeddingInput) (NoteEmbedding, error) {
	if input.NoteID <= 0 {
		return NoteEmbedding{}, fmt.Errorf("note id must be positive")
	}
	if input.Model == "" {
		return NoteEmbedding{}, fmt.Errorf("embedding model is required")
	}
	if len(input.Vector) == 0 {
		return NoteEmbedding{}, fmt.Errorf("embedding vector is required")
	}

	if _, err := s.GetNote(ctx, input.NoteID); err != nil {
		return NoteEmbedding{}, err
	}

	now := time.Now().UTC()
	vectorJSON, err := encodeVector(input.Vector)
	if err != nil {
		return NoteEmbedding{}, err
	}
	contentHash := hashText(input.Text)

	_, err = s.db.ExecContext(ctx, `
INSERT INTO note_embeddings (note_id, model, dimensions, vector_json, content_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(note_id, model) DO UPDATE SET
	dimensions = excluded.dimensions,
	vector_json = excluded.vector_json,
	content_hash = excluded.content_hash,
	updated_at = excluded.updated_at
`, input.NoteID, input.Model, len(input.Vector), vectorJSON, contentHash, formatTime(now), formatTime(now))
	if err != nil {
		return NoteEmbedding{}, err
	}

	return NoteEmbedding{
		NoteID:      input.NoteID,
		Model:       input.Model,
		Dimensions:  len(input.Vector),
		Vector:      input.Vector,
		ContentHash: contentHash,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) GetEmbedding(ctx context.Context, noteID int64, model string) (NoteEmbedding, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT note_id, model, dimensions, vector_json, content_hash, created_at, updated_at
FROM note_embeddings
WHERE note_id = ? AND model = ?
`, noteID, model)

	embedding, err := scanEmbedding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return NoteEmbedding{}, ErrEmbeddingNotFound
	}
	if err != nil {
		return NoteEmbedding{}, err
	}

	return embedding, nil
}

// ListEmbeddings returns all note embeddings created with one model.
// Vectors from different models must not be compared with each other.
func (s *Store) ListEmbeddings(ctx context.Context, model string) ([]NoteEmbedding, error) {
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT note_id, model, dimensions, vector_json, content_hash, created_at, updated_at
FROM note_embeddings
WHERE model = ?
ORDER BY note_id ASC
`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var embeddings []NoteEmbedding
	for rows.Next() {
		embedding, err := scanEmbedding(rows)
		if err != nil {
			return nil, err
		}
		embeddings = append(embeddings, embedding)
	}

	return embeddings, rows.Err()
}

func (s *Store) DeleteEmbeddings(ctx context.Context, noteID int64) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM note_embeddings
WHERE note_id = ?
`, noteID)
	return err
}

type embeddingScanner interface {
	Scan(dest ...any) error
}

func scanEmbedding(scanner embeddingScanner) (NoteEmbedding, error) {
	var embedding NoteEmbedding
	var vectorJSON string
	var createdAt string
	var updatedAt string

	if err := scanner.Scan(
		&embedding.NoteID,
		&embedding.Model,
		&embedding.Dimensions,
		&vectorJSON,
		&embedding.ContentHash,
		&createdAt,
		&updatedAt,
	); err != nil {
		return NoteEmbedding{}, err
	}

	vector, err := decodeVector(vectorJSON)
	if err != nil {
		return NoteEmbedding{}, err
	}
	if len(vector) != embedding.Dimensions {
		return NoteEmbedding{}, fmt.Errorf("embedding dimensions mismatch: metadata=%d vector=%d", embedding.Dimensions, len(vector))
	}

	embedding.Vector = vector
	embedding.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return NoteEmbedding{}, err
	}
	embedding.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return NoteEmbedding{}, err
	}

	return embedding, nil
}

func encodeVector(vector []float64) (string, error) {
	data, err := json.Marshal(vector)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeVector(value string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(value), &vector); err != nil {
		return nil, err
	}
	return vector, nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
