package version

import "runtime"

// Set via -ldflags at build time.
var (
	Version   = "0.9.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// GoVersion returns the Go runtime version.
func GoVersion() string {
	return runtime.Version()
}

// Info returns a map of version information.
func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"build_time": BuildTime,
		"git_commit": GitCommit,
		"go_version": GoVersion(),
	}
}
