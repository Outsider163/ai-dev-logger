package cli

import (
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
	Short: "Generate and store note embeddings",
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

		client := llm.NewClient(cfg.LLM)
		for _, note := range notes {
			if err := saveNoteEmbedding(cmd, db, client, cfg.LLM.EmbeddingModel, note); err != nil {
				return err
			}
		}
		if embedAll {
			fmt.Printf("rebuilt embeddings for %d notes\n", len(notes))
		}
		return nil
	},
}

var embedAll bool

func init() {
	embedCmd.Flags().BoolVar(&embedAll, "all", false, "Generate embeddings for every note")
}

func saveNoteEmbedding(cmd *cobra.Command, db *store.Store, client *llm.Client, model string, note store.Note) error {
	text := noteEmbeddingText(note)
	vector, err := client.CreateEmbedding(cmd.Context(), text)
	if err != nil {
		return fmt.Errorf("create embedding for note #%d: %w", note.ID, err)
	}

	embedding, err := db.UpsertEmbedding(cmd.Context(), store.UpsertEmbeddingInput{
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

func noteEmbeddingText(note store.Note) string {
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
