package cli

import (
	"testing"

	"ai-dev-logger/internal/store"
)

func TestSelectCurrentSemanticCandidates(t *testing.T) {
	currentNote := store.Note{ID: 1, Title: "Current", Body: "same body"}
	staleNote := store.Note{ID: 2, Title: "Stale", Body: "new body"}
	candidates := []store.EmbeddedNote{
		{
			Note: currentNote,
			Embedding: store.NoteEmbedding{
				NoteID:      currentNote.ID,
				ContentHash: testContentHash(store.NoteEmbeddingText(currentNote)),
			},
		},
		{
			Note: staleNote,
			Embedding: store.NoteEmbedding{
				NoteID:      staleNote.ID,
				ContentHash: testContentHash("old text"),
			},
		},
	}

	current, stale := selectCurrentSemanticCandidates(candidates)
	if stale != 1 {
		t.Fatalf("expected 1 stale candidate, got %d", stale)
	}
	if len(current) != 1 || current[0].Note.ID != currentNote.ID {
		t.Fatalf("expected only the current candidate, got %#v", current)
	}
}

func TestRankSemanticMatchesFiltersSortsAndSkipsInvalidVectors(t *testing.T) {
	candidates := []store.EmbeddedNote{
		{Note: store.Note{ID: 2, Title: "Second"}, Embedding: store.NoteEmbedding{Vector: []float64{0.8, 0.6}}},
		{Note: store.Note{ID: 1, Title: "First"}, Embedding: store.NoteEmbedding{Vector: []float64{1, 0}}},
		{Note: store.Note{ID: 3, Title: "Below threshold"}, Embedding: store.NoteEmbedding{Vector: []float64{-1, 0}}},
		{Note: store.Note{ID: 4, Title: "Wrong dimensions"}, Embedding: store.NoteEmbedding{Vector: []float64{1}}},
	}

	matches, skipped := rankSemanticMatches([]float64{1, 0}, candidates, 0.5)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %#v", matches)
	}
	if matches[0].note.ID != 1 || matches[1].note.ID != 2 {
		t.Fatalf("expected matches sorted by score, got %#v", matches)
	}
	if len(skipped) != 1 || skipped[0].noteID != 4 {
		t.Fatalf("expected note #4 to be skipped, got %#v", skipped)
	}
}
