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
	Use:     "add",
	Short:   "新增开发笔记",
	Long:    "新增带标题、正文和标签的笔记，正文支持 Markdown，也可从管道读取。\n默认只保存到本地；--ai 调用聊天接口整理内容，--embed 在保存后调用向量接口。",
	Example: "  adl add --title \"Go map\" --body \"写入前使用 make 初始化\" --tag go\n  adl add --title \"踩坑记录\" --body \"排查连接超时的过程\" --ai\n  Get-Content .\\note.md -Raw | adl add --title \"文件笔记\"",
	RunE: func(cmd *cobra.Command, args []string) error {
		title := strings.TrimSpace(addTitle)
		if title == "" {
			return fmt.Errorf("title is required; example: adl add --title \"Go map issue\" --body \"Use make before writing to a map\"")
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
			enhanced, err := llm.NewChatClient(cfg.ChatRuntimeProvider()).EnhanceNote(cmd.Context(), llm.EnhanceNoteInput{
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
			embeddingProvider := cfg.EmbeddingRuntimeProvider()
			embedding, err := saveNoteEmbedding(cmd.Context(), db, llm.NewEmbeddingClient(embeddingProvider), embeddingProvider.Model, note)
			if err != nil {
				return fmt.Errorf("note #%d was saved, but its embedding failed: %w", note.ID, err)
			}
			printSavedEmbedding(cmd.OutOrStdout(), embedding)
		}
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addTitle, "title", "t", "", "笔记标题")
	addCmd.Flags().StringVarP(&addBody, "body", "b", "", "笔记正文，支持 Markdown")
	addCmd.Flags().StringArrayVar(&addTags, "tag", nil, "标签，可重复传入")
	addCmd.Flags().BoolVar(&addAI, "ai", false, "调用聊天接口润色正文、生成摘要和标签")
	addCmd.Flags().BoolVar(&addEmbed, "embed", false, "保存笔记后调用向量接口生成向量")
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
