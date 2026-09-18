package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"ai-dev-logger/internal/store"
	"github.com/spf13/cobra"
)

const maxIngestFileBytes = 4 << 20

type ingestOptions struct {
	DBPath          string
	Paths           []string
	Tags            []string
	DuplicatePolicy string
	DryRun          bool
	Recursive       bool
	Extensions      []string
	Sync            bool
}

func newIngestCommand() *cobra.Command {
	options := ingestOptions{}
	command := &cobra.Command{
		Use:     "ingest <path> [path...]",
		Short:   "将 UTF-8 文本、Markdown 或代码文件导入知识库",
		Long:    "每个文件保存为一条笔记，显式文件以文件名为标题，目录文件以相对路径为标题。\n目录默认只扫描当前层；--recursive 扫描子目录，--ext 筛选扩展名。\n支持 UTF-8 普通文件，每个最多 4 MiB、每批最多 256 个；自动切片，不调用模型。\n全部文件校验通过后事务化导入，默认跳过相同内容。",
		Example: "  adl ingest notes.md main.go --tag project\n  adl ingest ./notes --recursive --dry-run\n  adl ingest ./notes --recursive --ext md,txt\n  adl embed --all\n  adl ask \"这些资料讲了什么？\"",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Sync && cmd.Flags().Changed("on-duplicate") {
				return fmt.Errorf("--sync cannot be combined with --on-duplicate")
			}
			options.DBPath, options.Paths = dbPath, args
			return runIngest(cmd.Context(), cmd.OutOrStdout(), options)
		},
	}
	command.Flags().StringSliceVar(&options.Tags, "tag", nil, "为导入的所有笔记添加标签，可重复传入")
	command.Flags().StringVar(&options.DuplicatePolicy, "on-duplicate", "skip", "重复策略：skip、error、allow")
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "预演导入，不保存笔记；可能初始化数据库")
	command.Flags().BoolVarP(&options.Recursive, "recursive", "r", false, "扫描目录的子目录，跳过隐藏项和常见构建/依赖目录")
	command.Flags().StringSliceVar(&options.Extensions, "ext", nil, "目录扫描的扩展名列表，例如 md,txt,go；不影响显式文件")
	command.Flags().BoolVar(&options.Sync, "sync", false, "按来源路径同步同一笔记；已有笔记保留标题/标签，正文冲突时报错")
	return command
}

func runIngest(ctx context.Context, output io.Writer, options ingestOptions) error {
	if len(options.Paths) == 0 || len(options.Paths) > 256 {
		return fmt.Errorf("pass between 1 and 256 paths")
	}
	policy, err := normalizeImportDuplicatePolicy(options.DuplicatePolicy)
	if err != nil {
		return err
	}
	files, err := collectIngestFiles(ctx, options)
	if err != nil {
		return err
	}
	inputs := make([]store.ImportNoteInput, 0, len(files))
	now := time.Now().UTC()
	total := 0
	for index, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		body, err := readIngestFile(file.Path)
		if err != nil {
			return fmt.Errorf("ingest %q: %w", file.Path, err)
		}
		total += len(body)
		if total > maxImportFileBytes {
			return fmt.Errorf("ingest batch exceeds 64 MiB")
		}
		inputs = append(inputs, store.ImportNoteInput{
			SourceID: int64(index + 1), Title: file.Title, Body: body,
			Tags: cleanTags(options.Tags), CreatedAt: now, UpdatedAt: now,
		})
	}
	if options.DryRun {
		for _, file := range files {
			if _, err := fmt.Fprintf(output, "file: %s -> %s\n", file.Path, file.Title); err != nil {
				return err
			}
		}
	}
	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if options.Sync {
		sources := make([]store.SyncNoteInput, len(inputs))
		for i, input := range inputs {
			sources[i] = store.SyncNoteInput{Path: files[i].Path, Title: input.Title, Body: input.Body, Tags: input.Tags}
		}
		result, err := db.SyncNotes(ctx, sources, options.DryRun)
		if err != nil {
			return err
		}
		prefix := "synced"
		if options.DryRun {
			prefix = "dry run"
		}
		if _, err := fmt.Fprintf(output, "%s: %d created, %d updated, %d unchanged\n", prefix, result.Created, result.Updated, result.Unchanged); err != nil {
			return err
		}
		if !options.DryRun && result.Created+result.Updated > 0 {
			_, err = fmt.Fprintln(output, "run: adl embed --all")
		}
		return err
	}
	result, err := db.ImportNotes(ctx, inputs, store.ImportNotesOptions{DuplicatePolicy: policy, DryRun: options.DryRun})
	if err != nil {
		return err
	}
	if options.DryRun {
		_, err = fmt.Fprintf(output, "dry run: would import %d note(s), skip %d duplicate(s)\n", result.Imported, result.Skipped)
		return err
	}
	_, err = fmt.Fprintf(output, "imported %d note(s), skipped %d duplicate(s)\n", result.Imported, result.Skipped)
	if err != nil {
		return err
	}
	if result.Imported > 0 {
		_, err = fmt.Fprintln(output, "run: adl embed --all; then: adl ask \"your question\"")
	}
	return err
}

func readIngestFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("expected a regular file")
	}
	if info.Size() > maxIngestFileBytes {
		return "", fmt.Errorf("file exceeds 4 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxIngestFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxIngestFileBytes {
		return "", fmt.Errorf("file exceeds 4 MiB")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("expected UTF-8 text without NUL bytes; convert the file to UTF-8 first")
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("file is empty")
	}
	return string(data), nil
}
