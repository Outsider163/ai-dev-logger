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
	"unicode"

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
		if err := ctx.Err(); err != nil {
			return err
		}
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

		command, argument, isCommand := classifyInteractiveInput(line)
		if isCommand {
			exit, err := runInteractiveCommand(ctx, output, db, scanner, command, argument)
			if err != nil {
				if exit {
					return err
				}
				fmt.Fprintf(output, "%s: %v\n", commandErrorPrefix, err)
				fmt.Fprintln(output, "输入 help 查看交互命令；要记录以命令词开头的文字，请使用 add 内容。")
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
	fmt.Fprintln(output, "输入 list 查看笔记，update 修改，delete 删除；输入 help 查看用法，exit 退出。")
	fmt.Fprintln(output, "兼容 /list 和 adl list；以命令词开头的笔记请用 add 内容。")
}

func splitInteractiveInput(line string) (string, string) {
	line = strings.TrimSpace(line)
	index := strings.IndexFunc(line, unicode.IsSpace)
	if index < 0 {
		return line, ""
	}
	return line[:index], strings.TrimSpace(line[index:])
}

func classifyInteractiveInput(line string) (string, string, bool) {
	token, argument := splitInteractiveInput(line)
	explicit := strings.HasPrefix(token, "/")
	command := strings.ToLower(strings.TrimPrefix(token, "/"))
	if command == "adl" || command == "ai-dev-logger" {
		explicit = true
		token, argument = splitInteractiveInput(argument)
		command = strings.ToLower(strings.TrimPrefix(token, "/"))
		if command == "" {
			command = "help"
		}
	}
	switch command {
	case "--help", "-h":
		return "help", argument, true
	case "add", "list", "ls", "find", "search", "show", "update", "delete", "help", "exit", "quit", "q":
		return command, argument, true
	// Reserve CLI-only commands so they produce guidance instead of becoming notes.
	case "backup", "completion", "config", "doctor", "embed", "export", "import", "restore", "semantic", "setup", "status", "version", "--version":
		return command, argument, true
	default:
		return command, argument, explicit
	}
}

func runInteractiveCommand(
	ctx context.Context,
	output io.Writer,
	db *store.Store,
	scanner *bufio.Scanner,
	command string,
	argument string,
) (bool, error) {
	if command == "add" {
		if argument == "--help" || argument == "-h" {
			printInteractiveHelp(output)
			return false, nil
		}
		if token, rest := splitInteractiveInput(argument); token == "--" {
			argument = rest
		} else if strings.HasPrefix(argument, "-") {
			return false, fmt.Errorf("交互模式请使用 add 内容；带 --title、--ai 等参数的录入请退出后在 PowerShell 执行")
		}
		return false, saveInteractiveNote(ctx, output, db, argument)
	}
	args, err := parseInteractiveArguments(argument)
	if err != nil {
		return false, err
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printInteractiveHelp(output)
		return false, nil
	}

	switch command {
	case "exit", "quit", "q":
		if len(args) != 0 {
			return false, fmt.Errorf("usage: exit")
		}
		return true, nil
	case "help":
		if len(args) != 0 {
			return false, fmt.Errorf("usage: help")
		}
		printInteractiveHelp(output)
		return false, nil
	case "list", "ls":
		limit, err := parseInteractiveListArgs(args)
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
		query, limit, err := parseInteractiveSearchArgs(args)
		if err != nil {
			return false, err
		}
		notes, err := db.SearchNotes(ctx, query, limit)
		if err != nil {
			return false, err
		}
		printInteractiveNotes(output, notes, "no matching notes")
		return false, nil
	case "show":
		id, err := interactiveNoteID(command, args)
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
	case "update":
		return false, updateInteractiveNote(ctx, output, db, args)
	case "delete":
		return deleteInteractiveNote(ctx, output, db, scanner, args)
	default:
		return false, fmt.Errorf("交互模式不支持命令 %q；输入 help 查看支持的命令，其他 adl 命令请先 exit 后在 PowerShell 执行", command)
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
		return 0, fmt.Errorf("id must be a positive number")
	}
	return id, nil
}

func printInteractiveHelp(output io.Writer) {
	fmt.Fprintln(output, "交互命令:")
	fmt.Fprintln(output, "  add <内容>                  明确保存笔记，适合以命令词开头的内容")
	fmt.Fprintln(output, "  list [数量]                 列出最近笔记，默认 10 条，最多 100 条")
	fmt.Fprintln(output, "  search <关键词>             搜索标题、正文和标签，也可用 find")
	fmt.Fprintln(output, "  show <编号>                 查看完整笔记")
	fmt.Fprintln(output, "  update <编号> --body \"正文\" 替换正文，也支持 --title、多个 --tag")
	fmt.Fprintln(output, "  delete <编号>               核对标题后输入 y 确认，默认取消，不支持 --yes")
	fmt.Fprintln(output, "  help                        显示帮助")
	fmt.Fprintln(output, "  exit                        退出交互模式")
	fmt.Fprintln(output, "兼容 /list、/find、/show、/update、/delete、/exit 和 adl 前缀，例如 adl list。")
	fmt.Fprintln(output, "list 和 search 支持 --limit；修改标签是整组替换，--tag= 清空标签。")
	fmt.Fprintln(output, "修改参数支持单/双引号；Windows 路径建议用单引号。不会执行系统命令或展开环境变量。")
	fmt.Fprintln(output, "add 后的内容不解析引号或选项，仍会提取 #标签；add -- 内容 可保存以 - 开头的文字。")
	fmt.Fprintln(output, "示例: 排查了连接超时 #network")
	fmt.Fprintln(output, "示例: add list 的用法 #cli")
	fmt.Fprintln(output, "笔记编号是固定 ID，删除后不复用；列表中的编号可能不连续。")
	fmt.Fprintln(output, "以上简写在 adl> 提示符后使用；返回系统终端后需加 adl，完整帮助用 adl --help。")
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
