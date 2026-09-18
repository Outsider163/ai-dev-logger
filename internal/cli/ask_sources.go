package cli

import (
	"unicode/utf8"

	"ai-dev-logger/internal/llm"
)

// Select a first source per note before adding more chunks, so one long note
// cannot consume the entire context while other relevant notes are available.
func selectAskSources(matches []semanticMatch, noteLimit, chunksPerNote, budget int) ([]llm.SearchNote, int) {
	var sources []llm.SearchNote
	counts := make(map[int64]int)
	selected := make(map[[2]int64]bool)
	used := 0
	add := func(match semanticMatch) bool {
		key := [2]int64{match.note.ID, int64(match.chunkIndex)}
		if selected[key] {
			return false
		}
		source := llm.SearchNote{ID: match.note.ID, Chunk: match.chunkIndex + 1, Score: match.score,
			Title: match.note.Title, Tags: match.note.Tags, Summary: match.note.Summary, Body: match.note.Body}
		cost := utf8.RuneCountInString(llm.KnowledgeSourceText(source))
		if cost > budget-used {
			return false
		}
		used += cost
		counts[source.ID]++
		selected[key] = true
		sources = append(sources, source)
		return true
	}
	for _, match := range matches {
		if len(counts) >= noteLimit {
			break
		}
		if counts[match.note.ID] == 0 {
			add(match)
		}
	}
	for _, match := range matches {
		if count := counts[match.note.ID]; count > 0 && count < chunksPerNote {
			add(match)
		}
	}
	return sources, used
}
