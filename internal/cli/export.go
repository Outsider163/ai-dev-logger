package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

const exportSchemaVersion = 1

var exportFormat string
var exportOutput string
var exportForce bool

type exportOptions struct {
	DBPath     string
	Format     string
	Output     string
	Force      bool
	ExportedAt time.Time
}

type jsonExportDocument struct {
	SchemaVersion int              `json:"schema_version"`
	ExportedAt    string           `json:"exported_at"`
	Notes         []jsonExportNote `json:"notes"`
}

type jsonExportNote struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	Summary   string   `json:"summary"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

var exportCmd = &cobra.Command{
	Use:     "export",
	Short:   "将全部笔记导出为 Markdown 或 JSON",
	Long:    "Markdown 适合阅读，JSON 可用于 adl import。导出文件不包含向量。\n默认拒绝覆盖已有文件；需要完整数据库备份请使用 adl backup。",
	Example: "  adl export --format markdown --output notes.md\n  adl export --format json --output notes.json",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExport(cmd.Context(), cmd.OutOrStdout(), exportOptions{
			DBPath: dbPath,
			Format: exportFormat,
			Output: exportOutput,
			Force:  exportForce,
		})
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "markdown", "导出格式：markdown 或 json")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "导出文件路径")
	exportCmd.Flags().BoolVar(&exportForce, "force", false, "覆盖已有导出文件")
	_ = exportCmd.MarkFlagRequired("output")
}

func runExport(ctx context.Context, writer io.Writer, options exportOptions) error {
	format, err := normalizeExportFormat(options.Format)
	if err != nil {
		return err
	}
	outputPath := strings.TrimSpace(options.Output)
	if outputPath == "" {
		return fmt.Errorf("output path is empty, pass --output")
	}
	if pathsReferToSameFile(sqliteDatabaseFilePath(options.DBPath), outputPath) {
		return fmt.Errorf("output path must not be the SQLite database: %s", outputPath)
	}

	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	notes, err := db.ListAllNotes(ctx)
	if err != nil {
		return err
	}

	exportedAt := options.ExportedAt
	if exportedAt.IsZero() {
		exportedAt = time.Now().UTC()
	}
	data, err := encodeNotesExport(notes, format, exportedAt)
	if err != nil {
		return err
	}
	if err := writeExportFile(outputPath, data, options.Force); err != nil {
		return err
	}

	fmt.Fprintf(writer, "exported %d note(s) to %s (%s)\n", len(notes), outputPath, format)
	return nil
}

func normalizeExportFormat(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown", "md":
		return "markdown", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported export format %q; use markdown or json", value)
	}
}

func encodeNotesExport(notes []store.Note, format string, exportedAt time.Time) ([]byte, error) {
	switch format {
	case "markdown":
		return renderMarkdownExport(notes, exportedAt), nil
	case "json":
		return renderJSONExport(notes, exportedAt)
	default:
		return nil, fmt.Errorf("unsupported normalized export format %q", format)
	}
}

func renderJSONExport(notes []store.Note, exportedAt time.Time) ([]byte, error) {
	exportedNotes := make([]jsonExportNote, 0, len(notes))
	for _, note := range notes {
		tags := append([]string{}, note.Tags...)
		exportedNotes = append(exportedNotes, jsonExportNote{
			ID:        note.ID,
			Title:     note.Title,
			Body:      note.Body,
			Tags:      tags,
			Summary:   note.Summary,
			CreatedAt: formatExportTime(note.CreatedAt),
			UpdatedAt: formatExportTime(note.UpdatedAt),
		})
	}

	document := jsonExportDocument{
		SchemaVersion: exportSchemaVersion,
		ExportedAt:    formatExportTime(exportedAt),
		Notes:         exportedNotes,
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode JSON export: %w", err)
	}
	return append(data, '\n'), nil
}

func renderMarkdownExport(notes []store.Note, exportedAt time.Time) []byte {
	var builder strings.Builder
	builder.WriteString("# ai-dev-logger notes\n\n")
	fmt.Fprintf(&builder, "- Exported at: `%s`\n", formatExportTime(exportedAt))
	fmt.Fprintf(&builder, "- Notes: `%d`\n", len(notes))

	if len(notes) == 0 {
		builder.WriteString("\nNo notes found.\n")
		return []byte(builder.String())
	}

	for _, note := range notes {
		fmt.Fprintf(&builder, "\n---\n\n## Note #%d: %s\n\n", note.ID, markdownSingleLine(note.Title, "Untitled"))
		fmt.Fprintf(&builder, "- Created: `%s`\n", formatExportTime(note.CreatedAt))
		fmt.Fprintf(&builder, "- Updated: `%s`\n", formatExportTime(note.UpdatedAt))
		if len(note.Tags) > 0 {
			fmt.Fprintf(&builder, "- Tags: %s\n", formatMarkdownTags(note.Tags))
		}

		if strings.TrimSpace(note.Summary) != "" {
			builder.WriteString("\n### Summary\n\n")
			writeMarkdownBlockquote(&builder, note.Summary)
		}

		builder.WriteString("\n### Content\n\n")
		body := strings.TrimRight(note.Body, "\r\n")
		if strings.TrimSpace(body) == "" {
			body = "(empty)"
		}
		builder.WriteString(body)
		builder.WriteByte('\n')
	}

	return []byte(builder.String())
}

func formatExportTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func markdownSingleLine(value string, fallback string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return fallback
	}
	return value
}

func formatMarkdownTags(tags []string) string {
	formatted := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = markdownSingleLine(strings.ReplaceAll(tag, "`", "'"), "")
		if tag != "" {
			formatted = append(formatted, "`"+tag+"`")
		}
	}
	return strings.Join(formatted, ", ")
}

func writeMarkdownBlockquote(builder *strings.Builder, value string) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimRight(value, "\r\n")
	for _, line := range strings.Split(value, "\n") {
		builder.WriteString("> ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
}

func writeExportFile(path string, data []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}

	info, statErr := os.Stat(path)
	targetExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect export file: %w", statErr)
	}
	if targetExists && info.IsDir() {
		return fmt.Errorf("output path is a directory: %s", path)
	}
	if targetExists && !force {
		return fmt.Errorf("output file already exists: %s (use --force to overwrite)", path)
	}
	if !force {
		return writeNewExportFile(path, data)
	}
	return replaceExportFile(path, data, targetExists)
}

func writeNewExportFile(path string, data []byte) (returnErr error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("output file already exists: %s (use --force to overwrite)", path)
	}
	if err != nil {
		return fmt.Errorf("open export file: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = os.Remove(path)
		}
	}()

	return writeAndCloseExportFile(file, data)
}

func replaceExportFile(path string, data []byte, targetExists bool) error {
	directory := filepath.Dir(path)
	tempFile, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary export file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if err := writeAndCloseExportFile(tempFile, data); err != nil {
		return err
	}
	if !targetExists {
		if err := os.Rename(tempPath, path); err != nil {
			return fmt.Errorf("install export file: %w", err)
		}
		return nil
	}

	// Unix can atomically replace the target. Windows falls back to a recoverable backup swap.
	if err := os.Rename(tempPath, path); err == nil {
		return nil
	}

	backupFile, err := os.CreateTemp(directory, "."+filepath.Base(path)+".backup-*")
	if err != nil {
		return fmt.Errorf("prepare export backup path: %w", err)
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("close export backup placeholder: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare export backup path: %w", err)
	}
	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("back up existing export file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		restoreErr := os.Rename(backupPath, path)
		if restoreErr != nil {
			return fmt.Errorf("replace export file: %v; restore previous file: %w", err, restoreErr)
		}
		return fmt.Errorf("replace export file: %w (previous file restored)", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("export written, but remove temporary backup %s: %w", backupPath, err)
	}
	return nil
}

func writeAndCloseExportFile(file *os.File, data []byte) error {
	written, err := file.Write(data)
	if err != nil {
		file.Close()
		return fmt.Errorf("write export file: %w", err)
	}
	if written != len(data) {
		file.Close()
		return fmt.Errorf("write export file: %w", io.ErrShortWrite)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync export file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close export file: %w", err)
	}
	return nil
}

func pathsReferToSameFile(first string, second string) bool {
	firstAbsolute, firstErr := comparableAbsolutePath(first)
	secondAbsolute, secondErr := comparableAbsolutePath(second)
	if firstErr == nil && secondErr == nil {
		if firstAbsolute == secondAbsolute || (runtime.GOOS == "windows" && strings.EqualFold(firstAbsolute, secondAbsolute)) {
			return true
		}
	}

	firstInfo, firstStatErr := os.Stat(first)
	secondInfo, secondStatErr := os.Stat(second)
	return firstStatErr == nil && secondStatErr == nil && os.SameFile(firstInfo, secondInfo)
}

func comparableAbsolutePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absolute)); err == nil {
		absolute = filepath.Join(resolvedParent, filepath.Base(absolute))
	}
	return filepath.Clean(absolute), nil
}

func sqliteDatabaseFilePath(path string) string {
	if separator := strings.Index(path, "?"); separator >= 0 {
		return path[:separator]
	}
	return path
}
