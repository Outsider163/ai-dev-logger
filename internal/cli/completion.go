package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var completionNoDescriptions bool

var completionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	Short:                 "生成终端命令补全脚本",
	Long:                  "支持 Bash、Zsh、Fish 和 PowerShell；脚本输出到标准输出。\n此命令不会自动修改终端配置。",
	Example:               "  adl completion powershell\n  adl completion powershell --no-descriptions",
	Args:                  cobra.ExactArgs(1),
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return writeShellCompletion(rootCmd, cmd.OutOrStdout(), args[0], completionNoDescriptions)
	},
}

func init() {
	completionCmd.Flags().BoolVar(
		&completionNoDescriptions,
		"no-descriptions",
		false,
		"补全建议中不显示命令说明",
	)
}

func writeShellCompletion(command *cobra.Command, writer io.Writer, shell string, noDescriptions bool) error {
	shell = strings.ToLower(strings.TrimSpace(shell))
	includeDescriptions := !noDescriptions

	var err error
	switch shell {
	case "bash":
		err = command.GenBashCompletionV2(writer, includeDescriptions)
	case "zsh":
		if includeDescriptions {
			err = command.GenZshCompletion(writer)
		} else {
			err = command.GenZshCompletionNoDesc(writer)
		}
	case "fish":
		err = command.GenFishCompletion(writer, includeDescriptions)
	case "powershell":
		if includeDescriptions {
			err = command.GenPowerShellCompletionWithDesc(writer)
		} else {
			err = command.GenPowerShellCompletion(writer)
		}
	default:
		return fmt.Errorf("unsupported shell %q; use bash, zsh, fish, or powershell", shell)
	}
	if err != nil {
		return fmt.Errorf("generate %s completion: %w", shell, err)
	}
	return nil
}
