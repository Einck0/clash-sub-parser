// Package platform provides foundational infrastructure, configuration, and runtime metadata for CSP 1.0.
package platform

// Build-time and runtime metadata for the CSP platform.
var (
	// Version is the current semantic version of CSP 1.0.
	Version = "1.0.0-clean-slate"

	// Commit is the git commit sha injected at build time.
	Commit = "dev"

	// BuildDate is the timestamp when the binary was built.
	BuildDate = "unknown"
)
