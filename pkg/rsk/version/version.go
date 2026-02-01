package version

var (
	// Version is the current version of RSK, injected at build time
	Version = "dev"
	// GitCommit is the git commit hash, injected at build time
	GitCommit = "unknown"
	// BuildDate is the build date, injected at build time
	BuildDate = "unknown"
)

// GetVersion returns the full version string
func GetVersion() string {
	return Version
}

// GetFullVersion returns the version with additional build information
func GetFullVersion() string {
	return "RSK " + Version + " (commit: " + GitCommit + ", built: " + BuildDate + ")"
}
