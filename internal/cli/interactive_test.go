package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-dev-logger/internal/store"
)

func TestRunInteractiveSavesAndReadsNotes(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "notes.db")
	input := strings.NewReader(strings.Join([]string{
		"fixed a Go map race #go #Concurrency #go",
		"/list",
		"/find map",
		"/show 1",
		"/exit",
	}, "\n") + "\n")
	var output bytes.Buffer

	if err := runInteractive(context.Background(), input, &output, databasePath); err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}

	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	note, err := db.GetNote(context.Background(), 1)
	if err != nil {
		t.Fatalf("get saved note: %v", err)
	}
	if note.Title != "fixed a Go map race" {
		t.Fatalf("unexpected title %q", note.Title)
	}
	if note.Body != "fixed a Go map race" {
		t.Fatalf("unexpected body %q", note.Body)
	}
	wantTags := []string{"go", "Concurrency"}
	if !reflect.DeepEqual(note.Tags, wantTags) {
		t.Fatalf("tags = %#v, want %#v", note.Tags, wantTags)
	}

	for _, expected := range []string{
		"saved note #1: fixed a Go map race",
		"tags: go, Concurrency",
		"#1  fixed a Go map race",
		"已退出交互模式",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestRunInteractiveReportsInputErrorsAndContinues(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "notes.db")
	input := strings.NewReader("/find\n/show nope\n/list 0\n/unknown\n/exit\n")
	var output bytes.Buffer

	if err := runInteractive(context.Background(), input, &output, databasePath); err != nil {
		t.Fatalf("runInteractive returned error: %v", err)
	}

	for _, expected := range []string{
		"操作失败: usage: search <query>",
		"操作失败: id must be a positive number",
		"操作失败: list limit must be between 1 and 100",
		"操作失败: 交互模式不支持命令 \"unknown\"",
		"已退出交互模式",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestInteractiveHelpExplainsCommandContext(t *testing.T) {
	var output bytes.Buffer
	printInteractiveWelcome(&output)
	printInteractiveHelp(&output)
	for _, expected := range []string{"本地交互模式", "不会调用 AI", "/list", "/find", "/show", "/exit", "adl> 提示符", "adl --help"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("interactive help is missing %q:\n%s", expected, output.String())
		}
	}
}

func TestRunQuickAddSavesParsedNote(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "notes.db")
	var output bytes.Buffer

	err := runQuickAdd(
		context.Background(),
		&output,
		databasePath,
		"fixed a SQLite lock #sqlite #Database",
	)
	if err != nil {
		t.Fatalf("runQuickAdd returned error: %v", err)
	}

	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	note, err := db.GetNote(context.Background(), 1)
	if err != nil {
		t.Fatalf("get saved note: %v", err)
	}
	if note.Title != "fixed a SQLite lock" || note.Body != "fixed a SQLite lock" {
		t.Fatalf("unexpected quick note: title=%q body=%q", note.Title, note.Body)
	}
	wantTags := []string{"sqlite", "Database"}
	if !reflect.DeepEqual(note.Tags, wantTags) {
		t.Fatalf("tags = %#v, want %#v", note.Tags, wantTags)
	}
	if !strings.Contains(output.String(), "saved note #1: fixed a SQLite lock") {
		t.Fatalf("unexpected output:\n%s", output.String())
	}
}

func TestNormalizeCommandLineArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "interactive", args: nil, want: nil},
		{name: "quoted note", args: []string{"fixed SQLite lock #sqlite"}, want: []string{quickAddCommandName, "fixed SQLite lock #sqlite"}},
		{name: "split note", args: []string{"fixed", "SQLite", "lock"}, want: []string{quickAddCommandName, "fixed", "SQLite", "lock"}},
		{name: "known command", args: []string{"list", "--limit", "5"}, want: []string{"list", "--limit", "5"}},
		{name: "database before note", args: []string{"--db", "notes.db", "hello"}, want: []string{"--db", "notes.db", quickAddCommandName, "hello"}},
		{name: "database equals", args: []string{"--db=notes.db", "hello"}, want: []string{"--db=notes.db", quickAddCommandName, "hello"}},
		{name: "note starting with dash", args: []string{"--", "-race condition"}, want: []string{quickAddCommandName, "--", "-race condition"}},
		{name: "help flag", args: []string{"--help"}, want: []string{"--help"}},
		{name: "version flag", args: []string{"--version"}, want: []string{"--version"}},
		{name: "completion protocol", args: []string{"__complete", "se"}, want: []string{"__complete", "se"}},
		{name: "unknown flag", args: []string{"--unknown", "hello"}, want: []string{"--unknown", "hello"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeCommandLineArgs(test.args)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalizeCommandLineArgs(%#v) = %#v, want %#v", test.args, got, test.want)
			}
		})
	}
}

func TestDeriveInteractiveTitleTruncatesByRunes(t *testing.T) {
	body := strings.Repeat("笔", 49)
	title := deriveInteractiveTitle(body)
	if title != strings.Repeat("笔", 48)+"..." {
		t.Fatalf("unexpected title %q", title)
	}
}
