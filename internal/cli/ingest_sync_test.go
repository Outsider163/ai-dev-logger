package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIngestSyncFileLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := writeIngestFixture(t, dir, "a.md", "first")
	options := ingestOptions{DBPath: filepath.Join(t.TempDir(), "notes.db"), Paths: []string{dir}, Sync: true}
	for _, step := range []struct {
		body, expected string
		dryRun         bool
	}{
		{"first", "1 created, 0 updated, 0 unchanged", false},
		{"first", "0 created, 0 updated, 1 unchanged", false},
		{"second", "0 created, 1 updated, 0 unchanged", true},
		{"second", "0 created, 1 updated, 0 unchanged", false},
	} {
		if err := os.WriteFile(path, []byte(step.body), 0600); err != nil {
			t.Fatal(err)
		}
		options.DryRun = step.dryRun
		var output bytes.Buffer
		if err := runIngest(context.Background(), &output, options); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), step.expected) {
			t.Fatal(output.String())
		}
		assertImportedNoteCount(t, options.DBPath, 1)
	}
}

func TestIngestSyncRejectsDuplicateFlag(t *testing.T) {
	command := newIngestCommand()
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs([]string{"unused.md", "--sync", "--on-duplicate", "skip"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("error = %v", err)
	}
}
