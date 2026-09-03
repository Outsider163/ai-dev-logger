package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"ai-dev-logger/internal/store"

	"github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
)

func parseInteractiveArguments(argument string) ([]string, error) {
	// Only tokenize input. Never expand secrets from the environment or run a shell.
	parser := &shellwords.Parser{ParseEnv: false, ParseBacktick: false, ParseComment: false}
	args, err := parser.Parse(argument)
	if err != nil {
		return nil, fmt.Errorf("参数格式不正确，请检查引号是否成对: %w", err)
	}
	if parser.Position >= 0 {
		return nil, fmt.Errorf("交互模式不执行管道、重定向或多条命令；正文中的特殊符号请放在引号内")
	}
	return args, nil
}

func interactiveFlags(name string) *pflag.FlagSet {
	flags := pflag.NewFlagSet(name, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseInteractiveListArgs(args []string) (int, error) {
	flags := interactiveFlags("list")
	limit := flags.Int("limit", interactiveListLimit, "maximum notes")
	if err := flags.Parse(args); err != nil {
		return 0, err
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && flags.Changed("limit")) {
		return 0, fmt.Errorf("usage: list [limit] or list --limit <limit>")
	}
	if flags.NArg() == 1 {
		return parseInteractiveLimit(flags.Arg(0))
	}
	return parseInteractiveLimit(strconv.Itoa(*limit))
}

func parseInteractiveSearchArgs(args []string) (string, int, error) {
	flags := interactiveFlags("search")
	limit := flags.Int("limit", interactiveListLimit, "maximum matches")
	if err := flags.Parse(args); err != nil {
		return "", 0, err
	}
	query := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if query == "" {
		return "", 0, fmt.Errorf("usage: search <query> [--limit <limit>]")
	}
	validatedLimit, err := parseInteractiveLimit(strconv.Itoa(*limit))
	return query, validatedLimit, err
}

func interactiveNoteID(command string, args []string) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("usage: %s <positive note id>", command)
	}
	return parseInteractiveID(args[0])
}

func updateInteractiveNote(ctx context.Context, output io.Writer, db *store.Store, args []string) error {
	flags := interactiveFlags("update")
	title := flags.StringP("title", "t", "", "new title")
	body := flags.StringP("body", "b", "", "complete new body")
	tags := flags.StringArray("tag", nil, "replacement tags")
	if err := flags.Parse(args); err != nil {
		return err
	}
	id, err := interactiveNoteID("update", flags.Args())
	if err != nil {
		return err
	}
	input := store.UpdateNoteInput{ID: id}
	if flags.Changed("title") {
		*title = strings.TrimSpace(*title)
		if *title == "" {
			return fmt.Errorf("title cannot be empty")
		}
		input.Title = title
	}
	if flags.Changed("body") {
		if strings.TrimSpace(*body) == "" {
			return fmt.Errorf("body cannot be empty")
		}
		input.Body = body
	}
	if flags.Changed("tag") {
		input.Tags = cleanTags(*tags)
		input.ReplaceTags = true
	}
	if input.Title == nil && input.Body == nil && !input.ReplaceTags {
		return fmt.Errorf("nothing to update, pass --title, --body, or --tag")
	}
	note, err := db.UpdateNote(ctx, input)
	if errors.Is(err, store.ErrNoteNotFound) {
		return fmt.Errorf("note #%d not found", id)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "updated note #%d\n", note.ID)
	return nil
}

func deleteInteractiveNote(ctx context.Context, output io.Writer, db *store.Store, scanner *bufio.Scanner, args []string) (bool, error) {
	id, err := interactiveNoteID("delete", args)
	if err != nil {
		return false, err
	}
	note, err := db.GetNote(ctx, id)
	if errors.Is(err, store.ErrNoteNotFound) {
		return false, fmt.Errorf("note #%d not found", id)
	}
	if err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(output, "即将永久删除笔记 #%d，标题: %q\n输入 y 确认删除，回车或其他输入取消 [y/N]: ", note.ID, note.Title); err != nil {
		return false, err
	}
	// Reuse the main scanner so a buffered confirmation cannot become another note.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return true, fmt.Errorf("read delete confirmation: %w", err)
		}
		fmt.Fprintln(output, "\n已取消删除")
		return true, nil
	}
	answer := strings.TrimSpace(scanner.Text())
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		fmt.Fprintln(output, "已取消删除；本次确认输入不会保存为笔记")
		return false, nil
	}
	if err := db.DeleteNote(ctx, id); errors.Is(err, store.ErrNoteNotFound) {
		return false, fmt.Errorf("note #%d not found", id)
	} else if err != nil {
		return false, err
	}
	fmt.Fprintf(output, "deleted note #%d\n", id)
	return false, nil
}
