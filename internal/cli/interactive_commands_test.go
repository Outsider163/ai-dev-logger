package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-dev-logger/internal/store"
)

func TestClassifyInteractiveInput(t *testing.T) {
	for _, test := range []struct {
		line, command, argument string
		isCommand               bool
	}{
		{"list", "list", "", true},
		{"/list 20", "list", "20", true},
		{"ADL\tLIST --limit 5", "list", "--limit 5", true},
		{"ai-dev-logger /show 6", "show", "6", true},
		{"adl", "help", "", true},
		{"adl --help", "help", "", true},
		{"/add list is useful", "add", "list is useful", true},
		{"update 6 --body 'new body'", "update", "6 --body 'new body'", true},
		{"/unknown text", "unknown", "text", true},
		{"adl unknown text", "unknown", "text", true},
		{"config show", "config", "show", true},
		{"today I learned Go", "today", "I learned Go", false},
		{"listing notes is useful", "listing", "notes is useful", false},
		{"这是 list 命令的说明", "这是", "list 命令的说明", false},
	} {
		t.Run(test.line, func(t *testing.T) {
			command, argument, isCommand := classifyInteractiveInput(test.line)
			if command != test.command || argument != test.argument || isCommand != test.isCommand {
				t.Fatalf("classification = %q, %q, %t; want %q, %q, %t", command, argument, isCommand, test.command, test.argument, test.isCommand)
			}
		})
	}
}

func TestInteractiveReservesPublicCLICommands(t *testing.T) {
	for _, command := range rootCmd.Commands() {
		if command.Hidden {
			continue
		}
		for _, name := range append([]string{command.Name()}, command.Aliases...) {
			if _, _, isCommand := classifyInteractiveInput(name); !isCommand {
				t.Errorf("CLI command %q would be saved as a note", name)
			}
		}
	}
}

func TestInteractiveCommandAliasesAreNotSavedAsNotes(t *testing.T) {
	for _, prefix := range []string{"", "/", "adl ", "ai-dev-logger "} {
		t.Run(prefix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.db")
			input := strings.Join([]string{
				"fixed a Go map race #go",
				prefix + "list", prefix + "list --limit 5", prefix + "ls 5",
				prefix + "search \"Go map\" --limit 5", prefix + "find map",
				prefix + "show 1", prefix + "help", prefix + "exit",
				"must not be saved after exit",
			}, "\n") + "\n"
			var output bytes.Buffer
			if err := runInteractive(context.Background(), strings.NewReader(input), &output, path); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), commandErrorPrefix) {
				t.Fatalf("commands failed:\n%s", output.String())
			}
			if strings.Count(output.String(), "#1  fixed a Go map race") != 6 {
				t.Fatalf("expected results from every read command:\n%s", output.String())
			}
			notes := readInteractiveTestNotes(t, path)
			if len(notes) != 1 || notes[0].Body != "fixed a Go map race" {
				t.Fatalf("commands were saved as notes: %#v", notes)
			}
		})
	}
}

func TestInteractiveAddEscapesCommandWordsAndPreservesText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.db")
	lines := []string{
		"add list 的用法 #cli", "/add delete 9", "adl add update is a command",
		"add add nested command", "add -- --title is literal text", "add -- /list",
		"add C:\\notes\\draft 'unmatched quote ; $ENV | not a command",
		"ordinary unbalanced ' note #go", "exit",
	}
	var output bytes.Buffer
	if err := runInteractive(context.Background(), strings.NewReader(strings.Join(lines, "\n")), &output, path); err != nil {
		t.Fatal(err)
	}
	notes := readInteractiveTestNotes(t, path)
	want := []string{
		"list 的用法", "delete 9", "update is a command", "add nested command",
		"--title is literal text", "/list", "C:\\notes\\draft 'unmatched quote ; $ENV | not a command",
		"ordinary unbalanced ' note",
	}
	if len(notes) != len(want) {
		t.Fatalf("notes = %d, want %d; output:\n%s", len(notes), len(want), output.String())
	}
	for i, note := range notes {
		if note.Body != want[i] {
			t.Fatalf("note %d body = %q, want %q", i, note.Body, want[i])
		}
	}
}

func TestInteractiveInvalidCommandsDoNotCreateNotes(t *testing.T) {
	commands := []string{
		"list nope", "/list 0", "list 101", "list --limit 0", "list 5 --limit 6", "list 1 2",
		"search", "search ''", "find --unknown foo", "search foo --limit 101",
		"show nope", "show 0", "show 1 2", "show 999",
		"update 1", "update 1 --body ''", "update 1 --title ' '", "update 1 --body 'unclosed",
		"update 1 --unknown x", "update 1 --body good extra", "update 999 --body good",
		"delete", "delete 1 2", "delete 1 --yes", "delete 999",
		"delete 1; show 1", "delete 1 && show 1", "list > output.txt", "list | show 1",
		"/unknown", "adl unknown", "config show", "adl setup", "adl --db other.db list",
		"add", "add #tag", "add --ai note", "add --title title --body body",
		"exit extra", "help extra",
	}
	path := filepath.Join(t.TempDir(), "notes.db")
	db := openInteractiveTestStore(t, path)
	note, err := db.CreateNote(context.Background(), store.CreateNoteInput{Title: "original title", Body: "ordinary note", Tags: []string{"old"}, Summary: "original summary"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertEmbedding(context.Background(), store.UpsertEmbeddingInput{
		NoteID: note.ID, Model: "test-model", Text: store.NoteEmbeddingText(note), Vector: []float64{1, 0},
	}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	input := strings.Join(commands, "\n") + "\nexit\n"
	if err := runInteractive(context.Background(), strings.NewReader(input), &output, path); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(output.String(), commandErrorPrefix+":"); got != len(commands) {
		t.Fatalf("got %d errors, want %d:\n%s", got, len(commands), output.String())
	}
	notes := readInteractiveTestNotes(t, path)
	if len(notes) != 1 || !reflect.DeepEqual(notes[0], note) {
		t.Fatalf("invalid commands mutated notes: %#v", notes)
	}
	if _, err := db.GetEmbedding(context.Background(), note.ID, "test-model"); err != nil {
		t.Fatalf("invalid commands removed an embedding: %v", err)
	}
}

func TestInteractiveUpdateUsesExistingStoreRules(t *testing.T) {
	for _, test := range []struct {
		name, command string
		title, body   string
		tags          []string
	}{
		{"title_only", `update 1 --title "new title"`, "new title", "old body", []string{"old"}},
		{"body_only", `/update 1 -b 'new body'`, "old title", "new body", []string{"old"}},
		{"replace_tags", `adl update 1 --tag go --tag sqlite --tag GO`, "old title", "old body", []string{"go", "sqlite"}},
		{"clear_tags", `update 1 --tag=`, "old title", "old body", []string{}},
		{"all_fields", `update 1 -t '新标题' --body="新的完整正文" --tag go`, "新标题", "新的完整正文", []string{"go"}},
		{"literal_path", `update 1 --body 'C:\notes\test.txt'`, "old title", `C:\notes\test.txt`, []string{"old"}},
		{"literal_shell", "update 1 --body '$ADL_TEST_SECRET `not-a-command` $(not-a-command) | > ; #literal'", "old title", "$ADL_TEST_SECRET `not-a-command` $(not-a-command) | > ; #literal", []string{"old"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ADL_TEST_SECRET", "must-not-expand")
			path := filepath.Join(t.TempDir(), "notes.db")
			db := openInteractiveTestStore(t, path)
			note, err := db.CreateNote(context.Background(), store.CreateNoteInput{
				Title: "old title", Body: "old body", Tags: []string{"old"}, Summary: "existing summary",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.UpsertEmbedding(context.Background(), store.UpsertEmbeddingInput{
				NoteID: note.ID, Model: "test-model", Text: store.NoteEmbeddingText(note), Vector: []float64{1, 0},
			}); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := runInteractive(context.Background(), strings.NewReader(test.command+"\nexit\n"), &output, path); err != nil {
				t.Fatal(err)
			}
			after, err := db.GetNote(context.Background(), note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Title != test.title || after.Body != test.body || !reflect.DeepEqual(after.Tags, test.tags) || after.Summary != note.Summary {
				t.Fatalf("unexpected updated note: %#v", after)
			}
			if _, err := db.GetEmbedding(context.Background(), note.ID, "test-model"); !errors.Is(err, store.ErrEmbeddingNotFound) {
				t.Fatalf("old embedding was not invalidated: %v", err)
			}
			if !strings.Contains(output.String(), "updated note #1") {
				t.Fatalf("missing update confirmation:\n%s", output.String())
			}
		})
	}
}

func TestInteractiveDeleteRequiresConfirmationAndKeepsIDsStable(t *testing.T) {
	for _, test := range []struct {
		command, answer string
	}{{"delete 1", "y"}, {"/delete 1", "y"}, {"adl delete 1", "y"}, {"delete 1", "YES"}, {"delete 1", " Y "}} {
		t.Run(test.command+"_"+test.answer, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.db")
			db := openInteractiveTestStore(t, path)
			note, err := db.CreateNote(context.Background(), store.CreateNoteInput{Title: "first note", Body: "first note"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.UpsertEmbedding(context.Background(), store.UpsertEmbeddingInput{
				NoteID: note.ID, Model: "test-model", Text: store.NoteEmbeddingText(note), Vector: []float64{1, 0},
			}); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			input := test.command + "\n" + test.answer + "\nsecond note\nexit\n"
			if err := runInteractive(context.Background(), strings.NewReader(input), &output, path); err != nil {
				t.Fatal(err)
			}
			notes := readInteractiveTestNotes(t, path)
			if len(notes) != 1 || notes[0].ID != 2 || notes[0].Body != "second note" {
				t.Fatalf("deleted IDs were reused or confirmation became a note: %#v", notes)
			}
			if _, err := db.GetEmbedding(context.Background(), note.ID, "test-model"); !errors.Is(err, store.ErrEmbeddingNotFound) {
				t.Fatalf("deleted note left an embedding: %v", err)
			}
			for _, expected := range []string{"即将永久删除笔记 #1", `标题: "first note"`, "[y/N]", "deleted note #1"} {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("delete is missing %q:\n%s", expected, output.String())
				}
			}
		})
	}
}

func TestInteractiveDeletePromptFailureDoesNotDelete(t *testing.T) {
	db := openInteractiveTestStore(t, filepath.Join(t.TempDir(), "notes.db"))
	note, err := db.CreateNote(context.Background(), store.CreateNoteInput{Title: "keep this note", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(strings.NewReader("y\n"))
	if _, err := deleteInteractiveNote(context.Background(), interactiveErrorWriter{}, db, scanner, []string{"1"}); err == nil {
		t.Fatal("expected a prompt write failure")
	}
	if _, err := db.GetNote(context.Background(), note.ID); err != nil {
		t.Fatalf("prompt failure deleted the note: %v", err)
	}
	if !scanner.Scan() || scanner.Text() != "y" {
		t.Fatal("prompt failure consumed confirmation")
	}
}

func TestInteractiveDeleteCancellationNeverSavesConfirmation(t *testing.T) {
	for _, answer := range []string{"", "n", "no", "yes please", "list", "/exit", "another note"} {
		t.Run(answer, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.db")
			var output bytes.Buffer
			input := "keep this note\ndelete 1\n" + answer + "\nlist\nexit\n"
			if err := runInteractive(context.Background(), strings.NewReader(input), &output, path); err != nil {
				t.Fatal(err)
			}
			notes := readInteractiveTestNotes(t, path)
			if len(notes) != 1 || notes[0].Body != "keep this note" {
				t.Fatalf("cancellation changed notes: %#v", notes)
			}
			if !strings.Contains(output.String(), "已取消删除") || !strings.Contains(output.String(), "#1  keep this note") {
				t.Fatalf("session did not continue after cancellation:\n%s", output.String())
			}
		})
	}
}

func TestInteractiveDeleteEOFAndReadFailureDoNotDelete(t *testing.T) {
	for _, test := range []struct {
		name   string
		broken bool
	}{{"eof", false}, {"read_error", true}} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes.db")
			var input io.Reader = strings.NewReader("keep this note\ndelete 1\n")
			if test.broken {
				input = io.MultiReader(input, interactiveErrorReader{})
			}
			var output bytes.Buffer
			err := runInteractive(context.Background(), input, &output, path)
			if test.broken && (err == nil || !strings.Contains(err.Error(), "read delete confirmation")) {
				t.Fatalf("expected confirmation read failure, got %v", err)
			}
			if !test.broken && err != nil {
				t.Fatal(err)
			}
			if notes := readInteractiveTestNotes(t, path); len(notes) != 1 {
				t.Fatalf("confirmation failure changed notes: %#v", notes)
			}
		})
	}
}

func TestInteractiveHelpDoesNotMutateNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.db")
	input := "adl\n--help\nlist --help\nshow --help\nupdate --help\ndelete --help\nadd --help\nexit\n"
	var output bytes.Buffer
	if err := runInteractive(context.Background(), strings.NewReader(input), &output, path); err != nil {
		t.Fatal(err)
	}
	if notes := readInteractiveTestNotes(t, path); len(notes) != 0 {
		t.Fatalf("help created notes: %#v", notes)
	}
	for _, expected := range []string{"update <编号>", "delete <编号>", "add list 的用法", "删除后不复用", "不会执行系统命令"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help missing %q", expected)
		}
	}
}

func TestInteractiveUpdateDoesNotConsumeFollowingInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.db")
	input := "first note #old\nupdate 1 --title 'new title'\nsecond note\nexit\n"
	if err := runInteractive(context.Background(), strings.NewReader(input), &bytes.Buffer{}, path); err != nil {
		t.Fatal(err)
	}
	notes := readInteractiveTestNotes(t, path)
	if len(notes) != 2 || notes[0].Body != "first note" || notes[0].Title != "new title" || notes[1].Body != "second note" {
		t.Fatalf("update consumed buffered session input: %#v", notes)
	}
}

func readInteractiveTestNotes(t *testing.T, path string) []store.Note {
	t.Helper()
	db := openInteractiveTestStore(t, path)
	notes, err := db.ListAllNotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func openInteractiveTestStore(t *testing.T, path string) *store.Store {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type interactiveErrorReader struct{}

func (interactiveErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("test input failure")
}

type interactiveErrorWriter struct{}

func (interactiveErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("test output failure")
}
