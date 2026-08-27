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
	Short:                 "Generate shell completion scripts",
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
		"Disable command descriptions in completion suggestions",
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
