package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"ai-dev-logger/internal/store"
)

const (
	interactiveListLimit = 10
	interactiveMaxLine   = 4 * 1024 * 1024
)

var interactiveTagPattern = regexp.MustCompile(`(^|[[:space:]])#([\p{L}\p{N}_-]+)`)

func runQuickAdd(ctx context.Context, output io.Writer, databasePath string, noteText string) error {
	if strings.TrimSpace(noteText) == "" {
		return fmt.Errorf("note text is required")
	}

	db, err := store.Open(databasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	return saveInteractiveNote(ctx, output, db, noteText)
}

func runInteractive(ctx context.Context, input io.Reader, output io.Writer, databasePath string) error {
	db, err := store.Open(databasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	printInteractiveWelcome(output)

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), interactiveMaxLine)

	for {
		fmt.Fprint(output, "adl> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read interactive input: %w", err)
			}
			fmt.Fprintln(output)
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			exit, err := runInteractiveCommand(ctx, output, db, line)
			if err != nil {
				fmt.Fprintf(output, "%s: %v\n", commandErrorPrefix, err)
				continue
			}
			if exit {
				fmt.Fprintln(output, "已退出交互模式")
				return nil
			}
			continue
		}

		if err := saveInteractiveNote(ctx, output, db, line); err != nil {
			fmt.Fprintf(output, "%s: %v\n", commandErrorPrefix, err)
		}
	}
}

func printInteractiveWelcome(output io.Writer) {
	fmt.Fprintln(output, "AI Dev Logger | 本地交互模式")
	fmt.Fprintln(output, "输入一行笔记后按 Enter 保存，#标签 会自动提取。这里不会调用 AI。")
	fmt.Fprintln(output, "输入 /help 查看命令，输入 /exit 退出。")
}

func runInteractiveCommand(
	ctx context.Context,
	output io.Writer,
	db *store.Store,
	line string,
) (bool, error) {
	commandToken, argument, _ := strings.Cut(line, " ")
	command := strings.ToLower(strings.TrimPrefix(commandToken, "/"))
	argument = strings.TrimSpace(argument)

	switch command {
	case "exit", "quit", "q":
		return true, nil
	case "help":
		printInteractiveHelp(output)
		return false, nil
	case "list", "ls":
		limit, err := parseInteractiveLimit(argument)
		if err != nil {
			return false, err
		}
		notes, err := db.ListNotes(ctx, limit)
		if err != nil {
			return false, err
		}
		printInteractiveNotes(output, notes, "no notes yet")
		return false, nil
	case "find", "search":
		if argument == "" {
			return false, fmt.Errorf("usage: /find <query>")
		}
		notes, err := db.SearchNotes(ctx, argument, interactiveListLimit)
		if err != nil {
			return false, err
		}
		printInteractiveNotes(output, notes, "no matching notes")
		return false, nil
	case "show":
		id, err := parseInteractiveID(argument)
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
		printInteractiveNote(output, note)
		return false, nil
	default:
		return false, fmt.Errorf("unknown command /%s; type /help", command)
	}
}

func saveInteractiveNote(ctx context.Context, output io.Writer, db *store.Store, input string) error {
	body, tags := parseInteractiveNote(input)
	if body == "" {
		return fmt.Errorf("note text is required")
	}

	note, err := db.CreateNote(ctx, store.CreateNoteInput{
		Title: deriveInteractiveTitle(body),
		Body:  body,
		Tags:  tags,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "saved note #%d: %s\n", note.ID, note.Title)
	if len(note.Tags) > 0 {
		fmt.Fprintf(output, "tags: %s\n", strings.Join(note.Tags, ", "))
	}
	return nil
}

func parseInteractiveNote(input string) (string, []string) {
	matches := interactiveTagPattern.FindAllStringSubmatch(input, -1)
	tags := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 3 {
			tags = append(tags, match[2])
		}
	}

	body := strings.TrimSpace(interactiveTagPattern.ReplaceAllString(input, "$1"))
	return body, cleanTags(tags)
}

func deriveInteractiveTitle(body string) string {
	title := strings.Join(strings.Fields(body), " ")
	const maxRunes = 48
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return string(runes[:maxRunes]) + "..."
}

func parseInteractiveLimit(argument string) (int, error) {
	if argument == "" {
		return interactiveListLimit, nil
	}
	limit, err := strconv.Atoi(argument)
	if err != nil || limit <= 0 || limit > 100 {
		return 0, fmt.Errorf("list limit must be between 1 and 100")
	}
	return limit, nil
}

func parseInteractiveID(argument string) (int64, error) {
	id, err := strconv.ParseInt(argument, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("usage: /show <positive note id>")
	}
	return id, nil
}

func printInteractiveHelp(output io.Writer) {
	fmt.Fprintln(output, "交互命令:")
	fmt.Fprintln(output, "  /list [limit]  列出最近笔记，默认 10 条，最多 100 条")
	fmt.Fprintln(output, "  /find <query>  按关键词搜索标题、标签和正文")
	fmt.Fprintln(output, "  /show <id>     查看一条笔记的完整内容")
	fmt.Fprintln(output, "  /help          显示帮助")
	fmt.Fprintln(output, "  /exit          退出交互模式")
	fmt.Fprintln(output, "示例: 排查了连接超时 #network")
	fmt.Fprintln(output, "交互命令只在 adl> 提示符后使用；返回系统终端后可运行 adl --help。")
}

func printInteractiveNotes(output io.Writer, notes []store.Note, emptyMessage string) {
	if len(notes) == 0 {
		fmt.Fprintln(output, emptyMessage)
		return
	}

	for _, note := range notes {
		fmt.Fprintf(output, "#%d  %s\n", note.ID, note.Title)
		if len(note.Tags) > 0 {
			fmt.Fprintf(output, "    tags: %s\n", strings.Join(note.Tags, ", "))
		}
		fmt.Fprintf(output, "    %s\n", firstLine(note.Body))
	}
}

func printInteractiveNote(output io.Writer, note store.Note) {
	fmt.Fprintf(output, "#%d  %s\n", note.ID, note.Title)
	fmt.Fprintf(output, "created: %s\n", note.CreatedAt.Format("2006-01-02 15:04"))
	if len(note.Tags) > 0 {
		fmt.Fprintf(output, "tags: %s\n", strings.Join(note.Tags, ", "))
	}
	if strings.TrimSpace(note.Summary) != "" {
		fmt.Fprintf(output, "summary: %s\n", note.Summary)
	}
	fmt.Fprintf(output, "\n%s\n", note.Body)
}
