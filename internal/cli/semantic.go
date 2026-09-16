package cli

import (
	"fmt"
	"math"
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
var semanticMinScore float64

type semanticMatch struct {
	chunkIndex int
	note       store.Note
	score      float64
}

type skippedSemanticMatch struct {
	noteID int64
	err    error
}

var semanticCmd = &cobra.Command{
	Use:     "semantic <query>",
	Short:   "按语义检索笔记，可附加 AI 解读",
	Long:    "需要先配置向量模型并运行 adl embed --all。\n查询时调用向量接口；--explain 还会把匹配笔记发送给聊天接口生成解读，可能产生 API 用量。",
	Example: "  adl semantic \"以前怎么解决数据库锁冲突\"\n  adl semantic \"连接超时如何排查\" --limit 3 --min-score 0.65\n  adl semantic \"Go map 的注意事项\" --explain",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(args[0])
		if query == "" {
			return fmt.Errorf("query is required")
		}
		if semanticLimit <= 0 {
			return fmt.Errorf("limit must be positive")
		}
		if math.IsNaN(semanticMinScore) || semanticMinScore < -1 || semanticMinScore > 1 {
			return fmt.Errorf("min-score must be between -1 and 1")
		}

		cfg, err := appconfig.Load(configPath)
		if err != nil {
			return err
		}
		embeddingProvider := cfg.EmbeddingRuntimeProvider()
		model := strings.TrimSpace(embeddingProvider.Model)
		if model == "" {
			return fmt.Errorf("embedding model is empty, run adl config set --embedding-model")
		}
		embeddingProvider.Model = model

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		candidates, err := db.ListEmbeddedNotes(cmd.Context(), model)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return fmt.Errorf("no embeddings found for model %q; run adl embed --all first", model)
		}

		currentCandidates, stale := selectCurrentSemanticCandidates(candidates)
		if stale > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped %d stale embeddings; run adl embed --all\n", stale)
		}
		if len(currentCandidates) == 0 {
			return fmt.Errorf("no current embeddings found for model %q; run adl embed --all first", model)
		}

		queryVector, err := llm.NewEmbeddingClient(embeddingProvider).CreateEmbedding(cmd.Context(), query)
		if err != nil {
			return fmt.Errorf("create query embedding: %w", err)
		}

		matches, invalid := rankSemanticMatches(queryVector, currentCandidates, semanticMinScore)
		for _, skipped := range invalid {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped note #%d: %v\n", skipped.noteID, skipped.err)
		}
		if len(invalid) == len(currentCandidates) {
			return fmt.Errorf("all stored embeddings are incompatible with the query vector; run adl embed --all --force")
		}
		if len(matches) == 0 {
			fmt.Printf("no semantic matches at or above %.4f\n", semanticMinScore)
			return nil
		}

		resultLimit := semanticLimit
		if resultLimit > len(matches) {
			resultLimit = len(matches)
		}
		selected := matches[:resultLimit]

		for _, match := range selected {
			fmt.Printf("#%d  %s  (similarity: %.4f)\n", match.note.ID, match.note.Title, match.score)
			fmt.Printf("    chunk: %d\n", match.chunkIndex+1)
			if len(match.note.Tags) > 0 {
				fmt.Printf("    tags: %s\n", strings.Join(match.note.Tags, ", "))
			}
			fmt.Printf("    %s\n\n", firstLine(match.note.Body))
		}

		if semanticExplain {
			contextNotes := make([]llm.SearchNote, 0, len(selected))
			for _, match := range selected {
				contextNotes = append(contextNotes, llm.SearchNote{
					ID:      match.note.ID,
					Chunk:   match.chunkIndex + 1,
					Score:   match.score,
					Title:   match.note.Title,
					Tags:    match.note.Tags,
					Summary: match.note.Summary,
					Body:    match.note.Body,
				})
			}

			explanation, err := llm.NewChatClient(cfg.ChatRuntimeProvider()).ExplainSearch(cmd.Context(), query, contextNotes)
			if err != nil {
				return fmt.Errorf("explain search results: %w", err)
			}
			fmt.Printf("AI explanation:\n%s\n", explanation)
		}
		return nil
	},
}

func init() {
	semanticCmd.Flags().IntVar(&semanticLimit, "limit", 5, "最多显示的匹配条数")
	semanticCmd.Flags().Float64Var(&semanticMinScore, "min-score", 0, "最低余弦相似度，范围为 -1 到 1")
	semanticCmd.Flags().BoolVar(&semanticExplain, "explain", false, "调用聊天接口解读匹配笔记")
}

func selectCurrentSemanticCandidates(candidates []store.EmbeddedNote) ([]store.EmbeddedNote, int) {
	current := make([]store.EmbeddedNote, 0, len(candidates))
	stale := 0
	for _, candidate := range candidates {
		if !candidate.Embedding.MatchesText(store.NoteEmbeddingText(candidate.Note)) {
			stale++
			continue
		}
		current = append(current, candidate)
	}
	return current, stale
}

func rankSemanticMatches(queryVector []float64, candidates []store.EmbeddedNote, minScore float64) ([]semanticMatch, []skippedSemanticMatch) {
	matches := make([]semanticMatch, 0, len(candidates))
	var skipped []skippedSemanticMatch
	for _, candidate := range candidates {
		score, err := semantic.CosineSimilarity(queryVector, candidate.Embedding.Vector)
		if err != nil {
			skipped = append(skipped, skippedSemanticMatch{noteID: candidate.Note.ID, err: err})
			continue
		}
		if score < minScore {
			continue
		}
		matches = append(matches, semanticMatch{note: candidate.Note, score: score, chunkIndex: candidate.Embedding.ChunkIndex})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			if matches[i].note.ID == matches[j].note.ID {
				return matches[i].chunkIndex < matches[j].chunkIndex
			}
			return matches[i].note.ID < matches[j].note.ID
		}
		return matches[i].score > matches[j].score
	})
	unique := make([]semanticMatch, 0, len(matches))
	seen := make(map[int64]bool)
	for _, match := range matches {
		if !seen[match.note.ID] {
			unique = append(unique, match)
			seen[match.note.ID] = true
		}
	}
	return unique, skipped
}
