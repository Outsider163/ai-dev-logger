package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"
)

type askOptions struct {
	ConfigPath string
	DBPath     string
	Question   string
	Limit      int
	MinScore   float64
	Output     io.Writer
	Error      io.Writer
}

func runAsk(ctx context.Context, options askOptions) error {
	question := strings.TrimSpace(options.Question)
	if question == "" {
		return fmt.Errorf("question is required")
	}
	if options.Limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	if math.IsNaN(options.MinScore) || options.MinScore < -1 || options.MinScore > 1 {
		return fmt.Errorf("min-score must be between -1 and 1")
	}
	cfg, err := appconfig.Load(options.ConfigPath)
	if err != nil {
		return err
	}
	embeddingProvider := cfg.EmbeddingRuntimeProvider()
	if embeddingProvider.Model == "" {
		return fmt.Errorf("embedding model is empty, run adl config set --embedding-model")
	}
	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	candidates, err := db.ListEmbeddedNotes(ctx, embeddingProvider.Model)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no embeddings found for model %q; run adl embed --all first", embeddingProvider.Model)
	}
	current, stale := selectCurrentSemanticCandidates(candidates)
	if stale > 0 {
		fmt.Fprintf(options.Error, "warning: skipped %d stale embeddings; run adl embed --all\n", stale)
	}
	if len(current) == 0 {
		return fmt.Errorf("no current embeddings found for model %q; run adl embed --all first", embeddingProvider.Model)
	}
	queryVector, err := llm.NewEmbeddingClient(embeddingProvider).CreateEmbedding(ctx, question)
	if err != nil {
		return fmt.Errorf("create question embedding: %w", err)
	}
	matches, invalid := rankSemanticMatches(queryVector, current, options.MinScore)
	for _, skipped := range invalid {
		fmt.Fprintf(options.Error, "warning: skipped note #%d: %v\n", skipped.noteID, skipped.err)
	}
	if len(invalid) == len(current) {
		return fmt.Errorf("all stored embeddings are incompatible with the question vector; run adl embed --all --force")
	}
	if len(matches) == 0 {
		fmt.Fprintln(options.Output, "没有找到足够相关的本地资料，因此未生成 AI 回答。")
		return nil
	}
	if options.Limit < len(matches) {
		matches = matches[:options.Limit]
	}
	sources := make([]llm.SearchNote, 0, len(matches))
	for _, match := range matches {
		sources = append(sources, llm.SearchNote{
			ID: match.note.ID, Chunk: match.chunkIndex + 1, Score: match.score,
			Title: match.note.Title, Tags: match.note.Tags, Summary: match.note.Summary, Body: match.note.Body,
		})
	}
	answer, err := llm.NewChatClient(cfg.ChatRuntimeProvider()).AnswerQuestion(ctx, question, sources)
	if err != nil {
		return fmt.Errorf("answer from knowledge base: %w", err)
	}
	fmt.Fprintln(options.Output, "Sources:")
	for _, source := range sources {
		fmt.Fprintf(options.Output, "[Note #%d / Chunk %d] %s (similarity: %.4f)\n", source.ID, source.Chunk, source.Title, source.Score)
	}
	fmt.Fprintf(options.Output, "\nAnswer:\n%s\n", answer)
	return nil
}
