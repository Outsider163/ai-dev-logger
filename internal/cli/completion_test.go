package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWriteShellCompletionSupportsAllShells(t *testing.T) {
	for shell, marker := range map[string]string{
		"bash":       "__start_sample",
		"zsh":        "#compdef sample",
		"fish":       "complete -c sample",
		"powershell": "Register-ArgumentCompleter",
	} {
		t.Run(shell, func(t *testing.T) {
			command := newCompletionTestCommand()
			var output bytes.Buffer
			if err := writeShellCompletion(command, &output, shell, false); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), marker) {
				t.Fatalf("expected %q in %s completion output, got:\n%s", marker, shell, output.String())
			}
		})
	}
}

func TestWriteShellCompletionNormalizesShellAndCanHideDescriptions(t *testing.T) {
	command := newCompletionTestCommand()
	var withDescriptions bytes.Buffer
	if err := writeShellCompletion(command, &withDescriptions, " PowerShell ", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withDescriptions.String(), "$Program __complete $Arguments") {
		t.Fatalf("expected completion requests with descriptions, got:\n%s", withDescriptions.String())
	}

	command = newCompletionTestCommand()
	var withoutDescriptions bytes.Buffer
	if err := writeShellCompletion(command, &withoutDescriptions, "POWERSHELL", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withoutDescriptions.String(), "$Program __completeNoDesc $Arguments") {
		t.Fatalf("expected completion requests without descriptions, got:\n%s", withoutDescriptions.String())
	}
}

func TestWriteShellCompletionRejectsUnsupportedShell(t *testing.T) {
	err := writeShellCompletion(newCompletionTestCommand(), &bytes.Buffer{}, "cmd", false)
	if err == nil || !strings.Contains(err.Error(), "bash, zsh, fish, or powershell") {
		t.Fatalf("expected unsupported shell error, got %v", err)
	}
}

func newCompletionTestCommand() *cobra.Command {
	command := &cobra.Command{Use: "sample"}
	command.AddCommand(&cobra.Command{
		Use:   "child",
		Short: "unique child description",
		Run:   func(cmd *cobra.Command, args []string) {},
	})
	return command
}
