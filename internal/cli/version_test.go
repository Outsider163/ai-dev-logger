package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"ai-dev-logger/internal/buildinfo"
)

func TestWriteVersion(t *testing.T) {
	var output bytes.Buffer
	writeVersion(&output)

	expected := []string{
		"version: " + buildinfo.Version,
		"commit: " + buildinfo.Commit,
		"built_at: " + buildinfo.BuiltAt,
		"go: " + runtime.Version(),
		"platform: " + runtime.GOOS + "/" + runtime.GOARCH,
	}
	for _, value := range expected {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("expected %q in version output, got %q", value, output.String())
		}
	}
}

func TestRootVersionMatchesBuildInfo(t *testing.T) {
	if rootCmd.Version != buildinfo.Version {
		t.Fatalf("root version %q does not match build version %q", rootCmd.Version, buildinfo.Version)
	}
}
