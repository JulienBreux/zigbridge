package version

var (
	// Version is the semantic release version or git descriptor.
	Version = "1.0.0"
	// Commit is the git commit sha.
	Commit = "none"
	// BuildDate is the RFC3339 UTC build timestamp.
	BuildDate = "unknown"
)

// Info returns a formatted version string with commit details when available.
func Info() string {
	if Commit != "" && Commit != "none" {
		return Version + " (" + Commit + ")"
	}
	return Version
}
