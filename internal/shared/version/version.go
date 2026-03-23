// Package version holds build-time version info injected via ldflags.
//
// Build with:
//
//	go build -ldflags "-X github.com/jaypaulb/trasker/internal/shared/version.Version=v1.0.0
//	  -X github.com/jaypaulb/trasker/internal/shared/version.Commit=abc1234
//	  -X github.com/jaypaulb/trasker/internal/shared/version.APIKey=key_...
//	  -X github.com/jaypaulb/trasker/internal/shared/version.ServerURL=https://..."
package version

import "fmt"

// These variables are set at build time via -ldflags -X.
var (
	// Version is the semantic version (e.g., "v1.0.0"). Default "dev" for local builds.
	Version = "dev"

	// Commit is the git commit hash. Default "unknown" for local builds.
	Commit = "unknown"

	// APIKey is the pre-baked API key for client binaries. Empty for server builds
	// and local dev builds. Set by the server build pipeline when generating
	// downloadable client binaries.
	APIKey = ""

	// ServerURL is the pre-baked server URL for client binaries. Empty for server
	// builds and local dev builds. Set by the server build pipeline.
	ServerURL = ""
)

// String returns a human-readable version string: "v1.0.0 (abc1234)".
func String() string {
	return fmt.Sprintf("%s (%s)", Version, Commit)
}
