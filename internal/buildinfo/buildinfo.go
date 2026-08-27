package buildinfo

// These values are replaced with -ldflags when a release binary is built.
var (
	Version = "dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)
