package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeIngestFixture(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCollectIngestDirectorySelection(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"README.md", "main.go", "sub/README.md", "sub/note.TXT", ".private.md", ".git/a.md", "node_modules/a.md", "dist/a.md", "image.png", "data.json"} {
		writeIngestFixture(t, dir, name, "content")
	}
	for _, tc := range []struct {
		name       string
		recursive  bool
		extensions []string
		want       []string
	}{
		{"top level", false, nil, []string{"README.md", "main.go"}},
		{"recursive", true, nil, []string{"README.md", "main.go", "sub/README.md", "sub/note.TXT"}},
		{"filtered", true, []string{".MD"}, []string{"README.md", "sub/README.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{dir, dir}, Recursive: tc.recursive, Extensions: tc.extensions})
			if err != nil {
				t.Fatal(err)
			}
			var titles []string
			for _, file := range files {
				titles = append(titles, file.Title)
			}
			if !reflect.DeepEqual(titles, tc.want) {
				t.Fatalf("titles = %v, want %v", titles, tc.want)
			}
		})
	}
}

func TestCollectIngestExplicitFileAndEmptySelection(t *testing.T) {
	dir := t.TempDir()
	path := writeIngestFixture(t, dir, "data.json", "{}")
	if _, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{dir}}); err == nil {
		t.Fatal("expected empty selection error")
	}
	files, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{path, path}, Extensions: []string{"md"}})
	if err != nil || len(files) != 1 {
		t.Fatalf("explicit file: %v, %v", files, err)
	}
	if _, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{dir}, Extensions: []string{"*.md"}}); err == nil {
		t.Fatal("expected invalid extension error")
	}
}

func TestDirectoryIngestPreviewsRelativePathsAndRollsBackInvalidBatch(t *testing.T) {
	dir := t.TempDir()
	writeIngestFixture(t, dir, "a.md", "first")
	bad := writeIngestFixture(t, dir, "sub/b.md", "invalid\x00text")
	dbPath := filepath.Join(t.TempDir(), "notes.db")
	options := ingestOptions{Paths: []string{dir}, DBPath: dbPath, Recursive: true, DryRun: true}
	var output bytes.Buffer
	if err := runIngest(context.Background(), &output, options); err == nil {
		t.Fatal("expected invalid file error")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("database was opened: %v", err)
	}
	if err := os.WriteFile(bad, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runIngest(context.Background(), &output, options); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "-> sub/b.md") {
		t.Fatal(output.String())
	}
	assertImportedNoteCount(t, dbPath, 0)
	options.DryRun = false
	if err := runIngest(context.Background(), &output, options); err != nil {
		t.Fatal(err)
	}
	assertImportedNoteCount(t, dbPath, 2)
}

func TestDirectoryIngestLimitAndCancellation(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 257; i++ {
		writeIngestFixture(t, dir, fmt.Sprintf("%03d.md", i), "text")
	}
	_, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{dir}})
	if err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("expected file limit, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := collectIngestFiles(ctx, ingestOptions{Paths: []string{dir}}); err != context.Canceled {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestCollectIngestSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := writeIngestFixture(t, t.TempDir(), "outside.md", "outside")
	link := filepath.Join(dir, "linked.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeIngestFixture(t, dir, "local.md", "local")
	files, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{dir}, Recursive: true})
	if err != nil || len(files) != 1 || files[0].Title != "local.md" {
		t.Fatalf("files = %v, err = %v", files, err)
	}
	if _, err := collectIngestFiles(context.Background(), ingestOptions{Paths: []string{link}}); err == nil {
		t.Fatal("explicit symlink accepted")
	}
}
