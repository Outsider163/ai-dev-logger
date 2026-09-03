package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func TestPublicCommandsHaveChineseHelpAndExamples(t *testing.T) {
	prepareHelp(rootCmd)
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Hidden {
			return
		}
		t.Run(command.CommandPath(), func(t *testing.T) {
			if !containsChinese(command.Short) {
				t.Fatalf("missing Chinese command description: %q", command.Short)
			}
			if !strings.Contains(command.Example, "adl") {
				t.Fatalf("missing short-command example: %q", command.Example)
			}
			for _, line := range strings.Split(command.LocalFlags().FlagUsages(), "\n") {
				if strings.TrimSpace(line) != "" && !containsChinese(line) {
					t.Fatalf("missing Chinese flag description: %q", line)
				}
			}

			var output bytes.Buffer
			previousOutput := command.OutOrStdout()
			command.SetOut(&output)
			defer command.SetOut(previousOutput)
			if err := command.Help(); err != nil {
				t.Fatal(err)
			}
			if !utf8.Valid(output.Bytes()) {
				t.Fatal("help output is not valid UTF-8")
			}
			for _, expected := range []string{"用法:", "示例:", "参数:", "--help"} {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("help is missing %q:\n%s", expected, output.String())
				}
			}
			for _, oldHeading := range []string{"Usage:", "Examples:", "Available Commands:", "Global Flags:", "help for "} {
				if strings.Contains(output.String(), oldHeading) {
					t.Fatalf("untranslated help text %q:\n%s", oldHeading, output.String())
				}
			}
		})
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(rootCmd)
}

func TestHelpRetainsImportantWarnings(t *testing.T) {
	for _, test := range []struct {
		command *cobra.Command
		warning string
	}{
		{rootCmd, "不需要 API Key"},
		{addCmd, "--ai"},
		{deleteCmd, "操作不可撤销"},
		{updateCmd, "替换整组标签"},
		{configShowCmd, "--reveal 会显示完整密钥"},
		{setupCmd, "本地配置文件"},
		{doctorCmd, "默认不联网"},
		{embedCmd, "API 用量"},
		{semanticCmd, "匹配笔记发送给聊天接口"},
		{restoreCmd, "替换目标数据库"},
		{backupCmd, "密钥不包含"},
		{exportCmd, "不包含向量"},
		{importCmd, "--dry-run"},
	} {
		t.Run(test.command.CommandPath(), func(t *testing.T) {
			if !strings.Contains(test.command.Long, test.warning) {
				t.Fatalf("help must explain %q: %s", test.warning, test.command.Long)
			}
		})
	}
}

func TestHelpEntryPointsDoNotRunCommands(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"help"}, {"help", "config", "set"},
		{"config", "--help"}, {"config", "set", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			calls := 0
			run := func(*cobra.Command, []string) { calls++ }
			root := &cobra.Command{Use: "sample", Short: "测试根命令", Version: "test", Run: run}
			config := &cobra.Command{Use: "config", Short: "测试配置命令", Run: run}
			config.AddCommand(&cobra.Command{Use: "set", Short: "测试配置写入", Run: run})
			root.AddCommand(config)
			prepareHelp(root)
			prepareHelp(root)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs(args)
			if _, err := root.ExecuteC(); err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatalf("help executed %d business handlers", calls)
			}
			if !strings.Contains(output.String(), "用法:") || strings.Contains(output.String(), "help for ") {
				t.Fatalf("unexpected help output:\n%s", output.String())
			}
		})
	}
}

func TestWriteCommandErrorProvidesScopedHelp(t *testing.T) {
	root := &cobra.Command{Use: "ai-dev-logger"}
	config := &cobra.Command{Use: "config"}
	set := &cobra.Command{Use: "set"}
	quickAdd := &cobra.Command{Use: quickAddCommandName, Hidden: true}
	config.AddCommand(set)
	root.AddCommand(config, quickAdd)
	root.SetArgs([]string{"config", "set", "--api-key", "do-not-repeat-this-secret"})
	for _, test := range []struct {
		name    string
		command *cobra.Command
		want    string
	}{
		{"root", root, "adl --help"},
		{"nested", set, "adl config set --help"},
		{"quick_add", quickAdd, "adl --help"},
		{"unknown", nil, "adl --help"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writeCommandError(&output, test.command, errors.New("invalid value"))
			want := "操作失败: invalid value\n查看用法: " + test.want + "\n"
			if output.String() != want {
				t.Fatalf("unexpected error output: %q, want %q", output.String(), want)
			}
		})
	}
	var output bytes.Buffer
	writeCommandError(&output, set, nil)
	if output.Len() != 0 {
		t.Fatalf("nil error must not print a failure: %q", output.String())
	}
}

func containsChinese(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
}
