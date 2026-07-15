package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestEmbeddingVectorCodec(t *testing.T) {
	vector := []float64{0.1, -0.2, 3.14}

	encoded, err := encodeVector(vector)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeVector(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(vector) {
		t.Fatalf("expected %d dimensions, got %d", len(vector), len(decoded))
	}
	for i := range vector {
		if decoded[i] != vector[i] {
			t.Fatalf("dimension %d: expected %v, got %v", i, vector[i], decoded[i])
		}
	}
}

func TestUpsertAndGetEmbedding(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{
		Title: "Embedding test",
		Body:  "first body",
		Tags:  []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "embedding-demo",
		Text:   note.Body,
		Vector: []float64{0.1, 0.2, 0.3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Dimensions != 3 {
		t.Fatalf("expected 3 dimensions, got %d", created.Dimensions)
	}

	got, err := db.GetEmbedding(ctx, note.ID, "embedding-demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentHash != hashText(note.Body) {
		t.Fatalf("unexpected content hash: %s", got.ContentHash)
	}
	if got.Vector[2] != 0.3 {
		t.Fatalf("unexpected vector: %#v", got.Vector)
	}

	updated, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  "embedding-demo",
		Text:   "updated body",
		Vector: []float64{0.9, 0.8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Dimensions != 2 {
		t.Fatalf("expected updated dimensions, got %d", updated.Dimensions)
	}

	got, err = db.GetEmbedding(ctx, note.ID, "embedding-demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dimensions != 2 || got.Vector[0] != 0.9 {
		t.Fatalf("expected updated embedding, got %#v", got)
	}
}

func TestUpsertEmbeddingRequiresExistingNote(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.UpsertEmbedding(ctx, UpsertEmbeddingInput{
		NoteID: 404,
		Model:  "embedding-demo",
		Text:   "missing note",
		Vector: []float64{0.1},
	})
	if !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
	}
}

func TestListEmbeddingsByModel(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, model := range []string{"model-a", "model-b", "model-a"} {
		note, err := db.CreateNote(ctx, CreateNoteInput{Title: model, Body: "body"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{NoteID: note.ID, Model: model, Text: "body", Vector: []float64{0.1}}); err != nil {
			t.Fatal(err)
		}
	}

	embeddings, err := db.ListEmbeddings(ctx, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
}

func TestUpdateNoteDeletesStaleEmbeddings(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "Old", Body: "old body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{NoteID: note.ID, Model: "model-a", Text: note.Body, Vector: []float64{0.1}}); err != nil {
		t.Fatal(err)
	}

	updatedBody := "new body"
	if _, err := db.UpdateNote(ctx, UpdateNoteInput{ID: note.ID, Body: &updatedBody}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetEmbedding(ctx, note.ID, "model-a"); !errors.Is(err, ErrEmbeddingNotFound) {
		t.Fatalf("expected stale embedding to be deleted, got %v", err)
	}
}

func TestDeleteNoteDeletesEmbeddings(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	note, err := db.CreateNote(ctx, CreateNoteInput{Title: "Delete", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{NoteID: note.ID, Model: "model-a", Text: note.Body, Vector: []float64{0.1}}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteNote(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	embeddings, err := db.ListEmbeddings(ctx, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(embeddings) != 0 {
		t.Fatalf("expected no orphan embeddings, got %d", len(embeddings))
	}
}

func TestGetEmbeddingStatus(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	first, err := db.CreateNote(ctx, CreateNoteInput{Title: "First", Body: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNote(ctx, CreateNoteInput{Title: "Second", Body: "two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(ctx, UpsertEmbeddingInput{NoteID: first.ID, Model: "model-a", Text: first.Body, Vector: []float64{0.1}}); err != nil {
		t.Fatal(err)
	}

	status, err := db.GetEmbeddingStatus(ctx, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if status.NotesTotal != 2 || status.EmbeddingsTotal != 1 || status.MissingForModel != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
}
