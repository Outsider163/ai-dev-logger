package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"ai-dev-logger/internal/store"

	"github.com/spf13/cobra"
)

const maxImportFileBytes = 64 << 20

var importInput string
var importDuplicatePolicy string
var importDryRun bool

type importOptions struct {
	DBPath          string
	Input           string
	DuplicatePolicy string
	DryRun          bool
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import notes from an ai-dev-logger JSON export",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runImport(cmd.Context(), cmd.OutOrStdout(), importOptions{
			DBPath:          dbPath,
			Input:           importInput,
			DuplicatePolicy: importDuplicatePolicy,
			DryRun:          importDryRun,
		})
	},
}

func init() {
	importCmd.Flags().StringVarP(&importInput, "input", "i", "", "JSON export file path")
	importCmd.Flags().StringVar(&importDuplicatePolicy, "on-duplicate", "skip", "Duplicate policy: skip, error, or allow")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Validate and preview without committing notes")
	_ = importCmd.MarkFlagRequired("input")
}

func runImport(ctx context.Context, writer io.Writer, options importOptions) error {
	inputPath := strings.TrimSpace(options.Input)
	if inputPath == "" {
		return fmt.Errorf("input path is empty, pass --input")
	}
	duplicatePolicy, err := normalizeImportDuplicatePolicy(options.DuplicatePolicy)
	if err != nil {
		return err
	}

	inputs, err := readJSONImport(inputPath)
	if err != nil {
		return err
	}

	db, err := store.Open(options.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.ImportNotes(ctx, inputs, store.ImportNotesOptions{
		DuplicatePolicy: duplicatePolicy,
		DryRun:          options.DryRun,
	})
	if err != nil {
		return err
	}

	if options.DryRun {
		fmt.Fprintf(
			writer,
			"dry run: would import %d note(s), skip %d duplicate(s) from %s\n",
			result.Imported,
			result.Skipped,
			inputPath,
		)
		return nil
	}

	fmt.Fprintf(
		writer,
		"imported %d note(s), skipped %d duplicate(s) from %s\n",
		result.Imported,
		result.Skipped,
		inputPath,
	)
	if result.Imported > 0 {
		fmt.Fprintln(writer, "embeddings are not imported; run: ai-dev-logger embed --all")
	}
	return nil
}

func normalizeImportDuplicatePolicy(value string) (store.DuplicatePolicy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "skip":
		return store.DuplicateSkip, nil
	case "error":
		return store.DuplicateError, nil
	case "allow":
		return store.DuplicateAllow, nil
	default:
		return "", fmt.Errorf("unsupported duplicate policy %q; use skip, error, or allow", value)
	}
}

func readJSONImport(path string) ([]store.ImportNoteInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open import file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxImportFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read import file: %w", err)
	}
	if len(data) > maxImportFileBytes {
		return nil, fmt.Errorf("import file exceeds %d bytes", maxImportFileBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document jsonExportDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode import JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode import JSON: multiple JSON values are not allowed")
		}
		return nil, fmt.Errorf("decode import JSON trailing data: %w", err)
	}

	return validateJSONImport(document)
}

func validateJSONImport(document jsonExportDocument) ([]store.ImportNoteInput, error) {
	if document.SchemaVersion != exportSchemaVersion {
		return nil, fmt.Errorf(
			"unsupported import schema_version %d; expected %d",
			document.SchemaVersion,
			exportSchemaVersion,
		)
	}
	if _, err := parseImportTime("exported_at", document.ExportedAt); err != nil {
		return nil, err
	}
	if document.Notes == nil {
		return nil, fmt.Errorf("import notes must be a JSON array")
	}

	inputs := make([]store.ImportNoteInput, 0, len(document.Notes))
	seenSourceIDs := make(map[int64]struct{}, len(document.Notes))
	for index, note := range document.Notes {
		position := index + 1
		if note.ID <= 0 {
			return nil, fmt.Errorf("import note at position %d has invalid id %d", position, note.ID)
		}
		if _, exists := seenSourceIDs[note.ID]; exists {
			return nil, fmt.Errorf("import file contains duplicate source id %d", note.ID)
		}
		seenSourceIDs[note.ID] = struct{}{}

		title := strings.TrimSpace(note.Title)
		if title == "" {
			return nil, fmt.Errorf("import note #%d title is empty", note.ID)
		}
		if strings.TrimSpace(note.Body) == "" {
			return nil, fmt.Errorf("import note #%d body is empty", note.ID)
		}
		if note.Tags == nil {
			return nil, fmt.Errorf("import note #%d tags must be a JSON array", note.ID)
		}

		createdAt, err := parseImportTime(fmt.Sprintf("import note #%d created_at", note.ID), note.CreatedAt)
		if err != nil {
			return nil, err
		}
		updatedAt, err := parseImportTime(fmt.Sprintf("import note #%d updated_at", note.ID), note.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if updatedAt.Before(createdAt) {
			return nil, fmt.Errorf("import note #%d updated_at is before created_at", note.ID)
		}

		inputs = append(inputs, store.ImportNoteInput{
			SourceID:  note.ID,
			Title:     title,
			Body:      note.Body,
			Tags:      cleanTags(note.Tags),
			Summary:   note.Summary,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	return inputs, nil
}

func parseImportTime(field string, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s is not a valid RFC3339 timestamp: %w", field, err)
	}
	return parsed.UTC(), nil
}
