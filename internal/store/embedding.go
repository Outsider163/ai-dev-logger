package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrEmbeddingNotFound = errors.New("embedding not found")

type NoteEmbedding struct {
	ChunkIndex  int
	NoteID      int64
	Model       string
	Dimensions  int
	Vector      []float64
	ContentHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EmbeddedNote carries note metadata, one chunk in Note.Body, and its vector.
type EmbeddedNote struct {
	Note      Note
	Embedding NoteEmbedding
}

// MatchesText reports whether the embedding was generated from text with the same content.
func (e NoteEmbedding) MatchesText(text string) bool {
	return e.ContentHash == hashText(text)
}

type UpsertEmbeddingInput struct {
	ChunkIndex int
	NoteID     int64
	Model      string
	Text       string
	Vector     []float64
}

// EmbeddingStatus describes whether notes are ready for semantic search.
type EmbeddingStatus struct {
	NotesTotal      int
	EmbeddingsTotal int
	CurrentForModel int
	MissingForModel int
	StaleForModel   int
	EmbeddingModel  string
}

func (s *Store) UpsertEmbedding(ctx context.Context, input UpsertEmbeddingInput) (NoteEmbedding, error) {
	if input.ChunkIndex < 0 {
		return NoteEmbedding{}, fmt.Errorf("chunk index must not be negative")
	}
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
INSERT INTO note_embeddings (note_id, chunk_index, model, dimensions, vector_json, content_hash, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(note_id, chunk_index, model) DO UPDATE SET
	dimensions = excluded.dimensions,
	vector_json = excluded.vector_json,
	content_hash = excluded.content_hash,
	updated_at = excluded.updated_at
`, input.NoteID, input.ChunkIndex, input.Model, len(input.Vector), vectorJSON, contentHash, formatTime(now), formatTime(now))
	if err != nil {
		return NoteEmbedding{}, err
	}

	return NoteEmbedding{
		ChunkIndex:  input.ChunkIndex,
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
	return s.GetChunkEmbedding(ctx, noteID, 0, model)
}

func (s *Store) GetChunkEmbedding(ctx context.Context, noteID int64, index int, model string) (NoteEmbedding, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT note_id, model, dimensions, vector_json, content_hash, created_at, updated_at, chunk_index
FROM note_embeddings
WHERE note_id = ? AND model = ? AND chunk_index = ?
`, noteID, model, index)

	embedding, err := scanEmbedding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return NoteEmbedding{}, ErrEmbeddingNotFound
	}
	if err != nil {
		return NoteEmbedding{}, err
	}

	return embedding, nil
}

// ListEmbeddings returns all chunk embeddings created with one model.
// Vectors from different models must not be compared with each other.
func (s *Store) ListEmbeddings(ctx context.Context, model string) ([]NoteEmbedding, error) {
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT note_id, model, dimensions, vector_json, content_hash, created_at, updated_at, chunk_index
FROM note_embeddings
WHERE model = ?
ORDER BY note_id ASC, chunk_index ASC
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

// ListEmbeddedNotes reads chunk bodies and their vectors with one joined query.
func (s *Store) ListEmbeddedNotes(ctx context.Context, model string) ([]EmbeddedNote, error) {
	if model == "" {
		return nil, fmt.Errorf("embedding model is required")
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT
	n.id, n.title, c.content, n.tags_json, n.summary, n.created_at, n.updated_at,
	e.note_id, e.model, e.dimensions, e.vector_json, e.content_hash, e.created_at, e.updated_at, e.chunk_index
FROM note_embeddings AS e
JOIN notes AS n ON n.id = e.note_id
JOIN note_chunks AS c ON c.note_id=e.note_id AND c.chunk_index=e.chunk_index
WHERE e.model = ?
ORDER BY n.id ASC, e.chunk_index ASC
`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []EmbeddedNote
	for rows.Next() {
		note, err := scanEmbeddedNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

// GetEmbeddingStatus classifies notes by completeness of their chunk vectors.
func (s *Store) GetEmbeddingStatus(ctx context.Context, model string) (EmbeddingStatus, error) {
	if model == "" {
		return EmbeddingStatus{}, fmt.Errorf("embedding model is required")
	}

	notes, err := s.ListAllNotes(ctx)
	if err != nil {
		return EmbeddingStatus{}, err
	}
	embeddings, err := s.ListEmbeddings(ctx, model)
	if err != nil {
		return EmbeddingStatus{}, err
	}

	status := EmbeddingStatus{
		NotesTotal:      len(notes),
		EmbeddingsTotal: len(embeddings),
		EmbeddingModel:  model,
	}
	embeddingsByNote := make(map[int64][]NoteEmbedding, len(embeddings))
	for _, embedding := range embeddings {
		embeddingsByNote[embedding.NoteID] = append(embeddingsByNote[embedding.NoteID], embedding)
	}

	for _, note := range notes {
		embedding, exists := embeddingsByNote[note.ID]
		if !exists {
			status.MissingForModel++
			continue
		}
		if !EmbeddingsCurrent(note, embedding, model) {
			status.StaleForModel++
			continue
		}
		status.CurrentForModel++
	}
	return status, nil
}

// NoteEmbeddingText builds the canonical text sent to the embeddings API.
func NoteEmbeddingText(note Note) string {
	var builder strings.Builder

	builder.WriteString("Title: ")
	builder.WriteString(note.Title)
	builder.WriteString("\n")

	if len(note.Tags) > 0 {
		builder.WriteString("Tags: ")
		builder.WriteString(strings.Join(note.Tags, ", "))
		builder.WriteString("\n")
	}

	if strings.TrimSpace(note.Summary) != "" {
		builder.WriteString("Summary: ")
		builder.WriteString(note.Summary)
		builder.WriteString("\n")
	}

	builder.WriteString("Body:\n")
	builder.WriteString(note.Body)

	return builder.String()
}

func (s *Store) DeleteEmbeddings(ctx context.Context, noteID int64) error {
	return deleteEmbeddings(ctx, s.db, noteID)
}

type embeddingExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func deleteEmbeddings(ctx context.Context, execer embeddingExecer, noteID int64) error {
	_, err := execer.ExecContext(ctx, `
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
		&embedding.ChunkIndex,
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

func scanEmbeddedNote(scanner embeddingScanner) (EmbeddedNote, error) {
	var item EmbeddedNote
	var tagsJSON string
	var noteCreatedAt string
	var noteUpdatedAt string
	var vectorJSON string
	var embeddingCreatedAt string
	var embeddingUpdatedAt string

	if err := scanner.Scan(
		&item.Note.ID,
		&item.Note.Title,
		&item.Note.Body,
		&tagsJSON,
		&item.Note.Summary,
		&noteCreatedAt,
		&noteUpdatedAt,
		&item.Embedding.NoteID,
		&item.Embedding.Model,
		&item.Embedding.Dimensions,
		&vectorJSON,
		&item.Embedding.ContentHash,
		&embeddingCreatedAt,
		&embeddingUpdatedAt,
		&item.Embedding.ChunkIndex,
	); err != nil {
		return EmbeddedNote{}, err
	}

	if err := json.Unmarshal([]byte(tagsJSON), &item.Note.Tags); err != nil {
		return EmbeddedNote{}, fmt.Errorf("decode tags for note %d: %w", item.Note.ID, err)
	}

	var err error
	item.Note.CreatedAt, err = parseTime(noteCreatedAt)
	if err != nil {
		return EmbeddedNote{}, err
	}
	item.Note.UpdatedAt, err = parseTime(noteUpdatedAt)
	if err != nil {
		return EmbeddedNote{}, err
	}

	item.Embedding.Vector, err = decodeVector(vectorJSON)
	if err != nil {
		return EmbeddedNote{}, err
	}
	if len(item.Embedding.Vector) != item.Embedding.Dimensions {
		return EmbeddedNote{}, fmt.Errorf(
			"embedding dimensions mismatch for note %d: metadata=%d vector=%d",
			item.Note.ID,
			item.Embedding.Dimensions,
			len(item.Embedding.Vector),
		)
	}
	item.Embedding.CreatedAt, err = parseTime(embeddingCreatedAt)
	if err != nil {
		return EmbeddedNote{}, err
	}
	item.Embedding.UpdatedAt, err = parseTime(embeddingUpdatedAt)
	if err != nil {
		return EmbeddedNote{}, err
	}

	return item, nil
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
