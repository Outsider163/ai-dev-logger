package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	appconfig "ai-dev-logger/internal/config"
	"ai-dev-logger/internal/llm"
	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

var addTitle string
var addBody string
var addTags []string
var addAI bool
var addEmbed bool

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a development note",
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.TrimSpace(addTitle)
		if title == "" {
			return fmt.Errorf("title is required, example: ai-dev-logger add --title \"Go map issue\"")
		}

		body, err := readBody()
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("body is required, pass --body or pipe content from stdin")
		}

		tags := cleanTags(addTags)
		summary := ""

		var cfg appconfig.Config
		if addAI || addEmbed {
			cfg, err = appconfig.Load(configPath)
			if err != nil {
				return err
			}
		}

		if addAI {
			enhanced, err := llm.NewClient(cfg.LLM).EnhanceNote(cmd.Context(), llm.EnhanceNoteInput{
				Title: title,
				Body:  body,
				Tags:  tags,
			})
			if err != nil {
				return fmt.Errorf("ai enhance note: %w", err)
			}

			if enhanced.Title != "" {
				title = enhanced.Title
			}
			if enhanced.Body != "" {
				body = enhanced.Body
			}
			summary = enhanced.Summary
			tags = cleanTags(append(tags, enhanced.Tags...))
		}

		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		note, err := db.CreateNote(cmd.Context(), store.CreateNoteInput{
			Title:   title,
			Body:    body,
			Tags:    tags,
			Summary: summary,
		})
		if err != nil {
			return err
		}

		fmt.Printf("saved note #%d\n", note.ID)
		if addEmbed {
			if err := saveNoteEmbedding(cmd, db, llm.NewClient(cfg.LLM), cfg.LLM.EmbeddingModel, note); err != nil {
				return fmt.Errorf("note #%d was saved, but its embedding failed: %w", note.ID, err)
			}
		}
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addTitle, "title", "t", "", "Note title")
	addCmd.Flags().StringVarP(&addBody, "body", "b", "", "Note body, Markdown is supported")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "Tag, can be used multiple times")
	addCmd.Flags().BoolVar(&addAI, "ai", false, "Use LLM to polish body, summarize, and generate tags")
	addCmd.Flags().BoolVar(&addEmbed, "embed", false, "Generate an embedding after the note is saved")
}

func readBody() (string, error) {
	if strings.TrimSpace(addBody) != "" {
		return addBody, nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cleanTags(tags []string) []string {
	seen := map[string]struct{}{}
	cleaned := make([]string, 0, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, tag)
	}

	return cleaned
}
