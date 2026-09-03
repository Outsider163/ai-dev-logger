package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

const commandErrorPrefix = "操作失败"

func prepareHelp(command *cobra.Command) {
	command.SetUsageTemplate(cliUsageTemplate)
	command.InitDefaultHelpCmd()
	prepareHelpFlags(command)
}

func prepareHelpFlags(command *cobra.Command) {
	command.InitDefaultHelpFlag()
	command.InitDefaultVersionFlag()
	command.Flags().Lookup("help").Usage = "显示此命令的帮助"
	if flag := command.Flags().Lookup("version"); flag != nil {
		flag.Usage = "显示程序版本"
	}
	for _, child := range command.Commands() {
		if child.Name() == "help" {
			child.Short = "查看命令帮助"
			child.Long = "查看命令和参数的详细用法；帮助操作不会修改笔记，也不会调用 AI。"
			child.Example = "  adl help\n  adl help add\n  adl config set --help"
		}
		prepareHelpFlags(child)
	}
}

func writeCommandError(writer io.Writer, command *cobra.Command, err error) {
	if err == nil {
		return
	}
	path := "adl"
	if command != nil && command.Name() != quickAddCommandName {
		path += strings.TrimPrefix(command.CommandPath(), command.Root().Name())
	}
	fmt.Fprintf(writer, "%s: %v\n查看用法: %s --help\n", commandErrorPrefix, err, path)
}

const cliUsageTemplate = `用法:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} <命令>{{end}}{{if gt (len .Aliases) 0}}

别名:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

可用命令:{{range .Commands}}{{if or .IsAvailableCommand (eq .Name "help")}}
  {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

参数:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局参数:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

使用 "{{.CommandPath}} <命令> --help" 查看详细用法。{{end}}
`
