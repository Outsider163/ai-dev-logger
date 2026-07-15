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
	Use:   "embed <id>",
	Short: "Generate and store an embedding for a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("id must be a positive number")
		}

		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		note, err := db.GetNote(cmd.Context(), id)
		if errors.Is(err, store.ErrNoteNotFound) {
			return fmt.Errorf("note #%d not found", id)
		}
		if err != nil {
			return err
		}

		text := noteEmbeddingText(note)
		vector, err := llm.NewClient(cfg.LLM).CreateEmbedding(cmd.Context(), text)
		if err != nil {
			return fmt.Errorf("create embedding: %w", err)
		}

		embedding, err := db.UpsertEmbedding(cmd.Context(), store.UpsertEmbeddingInput{
			NoteID: note.ID,
			Model:  cfg.LLM.EmbeddingModel,
			Text:   text,
			Vector: vector,
		})
		if err != nil {
			return err
		}

		fmt.Printf("saved embedding for note #%d using %s (%d dimensions)\n", embedding.NoteID, embedding.Model, embedding.Dimensions)
		return nil
	},
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
