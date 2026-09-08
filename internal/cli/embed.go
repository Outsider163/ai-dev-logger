package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var embedCmd = &cobra.Command{
	Use:     "embed [id]",
	Short:   "增量生成笔记向量",
	Long:    "调用向量接口处理笔记内容，并将结果存入 SQLite，可能产生 API 用量。\n默认跳过内容未变化的笔记；--force 强制重新生成。需要先配置向量模型。",
	Example: "  adl embed 1\n  adl embed --all\n  adl embed --all --force",
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
			return fmt.Errorf("embedding model is empty, run adl config set --embedding-model")
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
				cmd.Println("no notes to embed")
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
				cmd.Printf("embedding index is up to date: 0 generated, %d skipped, 0 failed\n", skipped)
			} else {
				cmd.Printf("embedding for note #%d is already up to date\n", notes[0].ID)
			}
			return nil
		}

		client := llm.NewClient(cfg.LLM)
		if err := client.ValidateEmbeddingConfig(); err != nil {
			return err
		}
		if !embedAll {
			embedding, err := saveNoteEmbedding(cmd.Context(), db, client, model, pending[0])
			if err != nil {
				return err
			}
			printSavedEmbedding(cmd.OutOrStdout(), embedding)
			return nil
		}

		result, batchErr := saveEmbeddingBatch(
			cmd.Context(),
			db,
			client,
			model,
			pending,
			cmd.OutOrStdout(),
			cmd.ErrOrStderr(),
		)
		cmd.Printf(
			"embedding index update finished: %d generated, %d skipped, %d failed\n",
			result.Generated,
			skipped,
			len(result.Failures),
		)
		if batchErr != nil {
			return batchErr
		}
		return result.failureError()
	},
}

var embedAll bool
var embedForce bool

func init() {
	embedCmd.Flags().BoolVar(&embedAll, "all", false, "更新全部笔记的向量")
	embedCmd.Flags().BoolVar(&embedForce, "force", false, "即使内容未变化也重新生成向量")
}

type embeddingFailure struct {
	NoteID int64
	Err    error
}

type embeddingBatchResult struct {
	Generated int
	Failures  []embeddingFailure
}

func saveNoteEmbedding(ctx context.Context, db *store.Store, client *llm.Client, model string, note store.Note) (store.NoteEmbedding, error) {
	parts := store.SplitNoteBody(note.Body)
	vectors := make([][]float64, 0, len(parts))
	for i, part := range parts {
		vector, err := client.CreateEmbedding(ctx, store.ChunkEmbeddingText(note, part))
		if err != nil {
			return store.NoteEmbedding{}, fmt.Errorf("create embedding for note #%d chunk %d/%d: %w", note.ID, i+1, len(parts), err)
		}
		vectors = append(vectors, vector)
	}
	embedding, err := db.SaveChunkEmbeddings(ctx, note, model, vectors)
	if err != nil {
		return store.NoteEmbedding{}, fmt.Errorf("store embedding for note #%d: %w", note.ID, err)
	}
	return embedding, nil
}

func saveEmbeddingBatch(
	ctx context.Context,
	db *store.Store,
	client *llm.Client,
	model string,
	notes []store.Note,
	stdout io.Writer,
	stderr io.Writer,
) (embeddingBatchResult, error) {
	result := embeddingBatchResult{Failures: make([]embeddingFailure, 0)}
	for index, note := range notes {
		current := index + 1
		fmt.Fprintf(stdout, "[%d/%d] embedding note #%d...\n", current, len(notes), note.ID)

		embedding, err := saveNoteEmbedding(ctx, db, client, model, note)
		if err != nil {
			result.Failures = append(result.Failures, embeddingFailure{NoteID: note.ID, Err: err})
			fmt.Fprintf(stderr, "[%d/%d] failed note #%d: %v\n", current, len(notes), note.ID, err)
			if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return result, err
			}
			continue
		}

		result.Generated++
		fmt.Fprintf(
			stdout,
			"[%d/%d] saved embedding for note #%d using %s (%d dimensions)\n",
			current,
			len(notes),
			embedding.NoteID,
			embedding.Model,
			embedding.Dimensions,
		)
	}
	return result, nil
}

func (r embeddingBatchResult) failureError() error {
	if len(r.Failures) == 0 {
		return nil
	}

	noteIDs := make([]string, 0, len(r.Failures))
	for _, failure := range r.Failures {
		noteIDs = append(noteIDs, fmt.Sprintf("#%d", failure.NoteID))
	}
	noteWord := "notes"
	if len(r.Failures) == 1 {
		noteWord = "note"
	}
	return fmt.Errorf(
		"%d %s failed to embed (%s); fix the reported errors and rerun adl embed --all",
		len(r.Failures),
		noteWord,
		strings.Join(noteIDs, ", "),
	)
}

func printSavedEmbedding(writer io.Writer, embedding store.NoteEmbedding) {
	fmt.Fprintf(
		writer,
		"saved embedding for note #%d using %s (%d dimensions)\n",
		embedding.NoteID,
		embedding.Model,
		embedding.Dimensions,
	)
}

func selectNotesForEmbedding(notes []store.Note, embeddings []store.NoteEmbedding, model string, force bool) ([]store.Note, int) {
	if force {
		return notes, 0
	}

	embeddingsByNote := make(map[int64][]store.NoteEmbedding, len(embeddings))
	for _, embedding := range embeddings {
		if embedding.Model == model {
			embeddingsByNote[embedding.NoteID] = append(embeddingsByNote[embedding.NoteID], embedding)
		}
	}

	pending := make([]store.Note, 0, len(notes))
	skipped := 0
	for _, note := range notes {
		embedding, exists := embeddingsByNote[note.ID]
		if exists && store.EmbeddingsCurrent(note, embedding, model) {
			skipped++
			continue
		}
		pending = append(pending, note)
	}
	return pending, skipped
}
