package cli

import (
	"fmt"
	"io"
	"runtime"

	"ai-dev-logger/internal/buildinfo"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "显示版本和构建信息",
	Example: "  adl version\n  adl --version",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		writeVersion(cmd.OutOrStdout())
	},
}

func writeVersion(writer io.Writer) {
	fmt.Fprintf(writer, "version: %s\n", buildinfo.Version)
	fmt.Fprintf(writer, "commit: %s\n", buildinfo.Commit)
	fmt.Fprintf(writer, "built_at: %s\n", buildinfo.BuiltAt)
	fmt.Fprintf(writer, "go: %s\n", runtime.Version())
	fmt.Fprintf(writer, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
