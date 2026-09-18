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
	ConfigPath    string
	DBPath        string
	Question      string
	Limit         int
	MinScore      float64
	Output        io.Writer
	Error         io.Writer
	ChunksPerNote int
	ContextChars  int
	RetrieveOnly  bool
}

func runAsk(ctx context.Context, options askOptions) error {
	if err := validateAskOptions(options); err != nil {
		return err
	}
	question := strings.TrimSpace(options.Question)
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
	matches, invalid := rankSemanticChunks(queryVector, current, options.MinScore)
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
	sources, usedChars := selectAskSources(matches, options.Limit, options.ChunksPerNote, options.ContextChars)
	if len(sources) == 0 {
		return fmt.Errorf("no source fits the context budget; increase --context-chars")
	}
	if options.RetrieveOnly {
		if err := printAskSources(options.Output, sources, usedChars, options.ContextChars); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(options.Output, "\nRetrieved context (chat API not called):"); err != nil {
			return err
		}
		for _, source := range sources {
			if _, err := io.WriteString(options.Output, llm.KnowledgeSourceText(source)); err != nil {
				return err
			}
		}
		return nil
	}
	answer, err := llm.NewChatClient(cfg.ChatRuntimeProvider()).AnswerQuestion(ctx, question, sources)
	if err != nil {
		return fmt.Errorf("answer from knowledge base: %w", err)
	}
	if err := printAskSources(options.Output, sources, usedChars, options.ContextChars); err != nil {
		return err
	}
	_, err = fmt.Fprintf(options.Output, "\nAnswer:\n%s\n", answer)
	return err
}

func printAskSources(output io.Writer, sources []llm.SearchNote, usedChars, budget int) error {
	if _, err := fmt.Fprintf(output, "Sources:\ncontext: %d chunks, %d/%d characters\n", len(sources), usedChars, budget); err != nil {
		return err
	}
	for _, source := range sources {
		if _, err := fmt.Fprintf(output, "[Note #%d / Chunk %d] %s (similarity: %.4f)\n", source.ID, source.Chunk, source.Title, source.Score); err != nil {
			return err
		}
	}
	return nil
}

func validateAskOptions(options askOptions) error {
	if strings.TrimSpace(options.Question) == "" {
		return fmt.Errorf("question is required")
	}
	if options.Limit <= 0 || options.Limit > 20 {
		return fmt.Errorf("limit must be between 1 and 20")
	}
	if options.ChunksPerNote < 1 || options.ChunksPerNote > 5 {
		return fmt.Errorf("chunks-per-note must be between 1 and 5")
	}
	if options.ContextChars < 2400 || options.ContextChars > 48000 {
		return fmt.Errorf("context-chars must be between 2400 and 48000")
	}
	if math.IsNaN(options.MinScore) || options.MinScore < -1 || options.MinScore > 1 {
		return fmt.Errorf("min-score must be between -1 and 1")
	}
	return nil
}
