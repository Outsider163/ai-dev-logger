package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var embedCmd = &cobra.Command{
	Use:   "embed [id]",
	Short: "Incrementally generate and store note embeddings",
	Args: func(cmd *cobra.Command, args []string) error {
		if embedAll && len(args) != 0 {
			return fmt.Errorf("--all does not accept a note id")
		}
		if !embedAll && len(args) != 1 {
			return fmt.Errorf("pass a note id or use --all")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}
		model := strings.TrimSpace(cfg.LLM.EmbeddingModel)
		if model == "" {
			return fmt.Errorf("embedding model is empty, run config set --embedding-model")
		}
		cfg.LLM.EmbeddingModel = model

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		var notes []store.Note
		if embedAll {
			notes, err = db.ListAllNotes(cmd.Context())
			if err != nil {
				return err
			}
			if len(notes) == 0 {
				fmt.Println("no notes to embed")
				return nil
			}
		} else {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("id must be a positive number")
			}
			note, err := db.GetNote(cmd.Context(), id)
			if errors.Is(err, store.ErrNoteNotFound) {
				return fmt.Errorf("note #%d not found", id)
			}
			if err != nil {
				return err
			}
			notes = []store.Note{note}
		}

		var existing []store.NoteEmbedding
		if !embedForce {
			existing, err = db.ListEmbeddings(cmd.Context(), model)
			if err != nil {
				return err
			}
		}

		pending, skipped := selectNotesForEmbedding(notes, existing, model, embedForce)
		if len(pending) == 0 {
			if embedAll {
				fmt.Printf("embedding index is up to date: 0 generated, %d skipped\n", skipped)
			} else {
				fmt.Printf("embedding for note #%d is already up to date\n", notes[0].ID)
			}
			return nil
		}

		client := llm.NewClient(cfg.LLM)
		for _, note := range pending {
			if err := saveNoteEmbedding(cmd.Context(), db, client, model, note); err != nil {
				return err
			}
		}
		if embedAll {
			fmt.Printf("embedding index updated: %d generated, %d skipped\n", len(pending), skipped)
		}
		return nil
	},
}

var embedAll bool
var embedForce bool

func init() {
	embedCmd.Flags().BoolVar(&embedAll, "all", false, "Update embeddings for all notes")
	embedCmd.Flags().BoolVar(&embedForce, "force", false, "Regenerate embeddings even when note content is unchanged")
}

func saveNoteEmbedding(ctx context.Context, db *store.Store, client *llm.Client, model string, note store.Note) error {
	text := store.NoteEmbeddingText(note)
	vector, err := client.CreateEmbedding(ctx, text)
	if err != nil {
		return fmt.Errorf("create embedding for note #%d: %w", note.ID, err)
	}

	embedding, err := db.UpsertEmbedding(ctx, store.UpsertEmbeddingInput{
		NoteID: note.ID,
		Model:  model,
		Text:   text,
		Vector: vector,
	})
	if err != nil {
		return err
	}

	fmt.Printf("saved embedding for note #%d using %s (%d dimensions)\n", embedding.NoteID, embedding.Model, embedding.Dimensions)
	return nil
}

func selectNotesForEmbedding(notes []store.Note, embeddings []store.NoteEmbedding, model string, force bool) ([]store.Note, int) {
	if force {
		return notes, 0
	}

	embeddingsByNote := make(map[int64]store.NoteEmbedding, len(embeddings))
	for _, embedding := range embeddings {
		if embedding.Model == model {
			embeddingsByNote[embedding.NoteID] = embedding
		}
	}

	pending := make([]store.Note, 0, len(notes))
	skipped := 0
	for _, note := range notes {
		embedding, exists := embeddingsByNote[note.ID]
		if exists && embedding.MatchesText(store.NoteEmbeddingText(note)) {
			skipped++
			continue
		}
		pending = append(pending, note)
	}
	return pending, skipped
}
