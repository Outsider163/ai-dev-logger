package cli

import (
	"fmt"
	"sort"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/semantic"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var semanticLimit int
var semanticExplain bool

type semanticMatch struct {
	note  store.Note
	score float64
}

var semanticCmd = &cobra.Command{
	Use:   "semantic <query>",
	Short: "Search notes by vector similarity",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(args[0])
		if query == "" {
			return fmt.Errorf("query is required")
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

		queryVector, err := llm.NewClient(cfg.LLM).CreateEmbedding(cmd.Context(), query)
		if err != nil {
			return fmt.Errorf("create query embedding: %w", err)
		}

		embeddings, err := db.ListEmbeddings(cmd.Context(), cfg.LLM.EmbeddingModel)
		if err != nil {
			return err
		}
		if len(embeddings) == 0 {
			return fmt.Errorf("no embeddings found for model %q; run embed <id> first", cfg.LLM.EmbeddingModel)
		}

		matches := make([]semanticMatch, 0, len(embeddings))
		for _, embedding := range embeddings {
			score, err := semantic.CosineSimilarity(queryVector, embedding.Vector)
			if err != nil {
				return fmt.Errorf("compare note #%d: %w", embedding.NoteID, err)
			}

			note, err := db.GetNote(cmd.Context(), embedding.NoteID)
			if err != nil {
				return err
			}
			matches = append(matches, semanticMatch{note: note, score: score})
		}

		sort.Slice(matches, func(i, j int) bool {
			return matches[i].score > matches[j].score
		})
		if semanticLimit <= 0 || semanticLimit > len(matches) {
			semanticLimit = len(matches)
		}

		for _, match := range matches[:semanticLimit] {
			fmt.Printf("#%d  %s  (similarity: %.4f)\n", match.note.ID, match.note.Title, match.score)
			if len(match.note.Tags) > 0 {
				fmt.Printf("    tags: %s\n", strings.Join(match.note.Tags, ", "))
			}
			fmt.Printf("    %s\n\n", firstLine(match.note.Body))
		}

		if semanticExplain {
			contextNotes := make([]llm.SearchNote, 0, semanticLimit)
			for _, match := range matches[:semanticLimit] {
				contextNotes = append(contextNotes, llm.SearchNote{
					Title:   match.note.Title,
					Tags:    match.note.Tags,
					Summary: match.note.Summary,
					Body:    match.note.Body,
				})
			}

			explanation, err := llm.NewClient(cfg.LLM).ExplainSearch(cmd.Context(), query, contextNotes)
			if err != nil {
				return fmt.Errorf("explain search results: %w", err)
			}
			fmt.Printf("AI explanation:\n%s\n", explanation)
		}
		return nil
	},
}

func init() {
	semanticCmd.Flags().IntVar(&semanticLimit, "limit", 5, "Maximum number of matches to show")
	semanticCmd.Flags().BoolVar(&semanticExplain, "explain", false, "Ask the chat model to explain the matching notes")
}
