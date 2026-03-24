package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Target represents a cross-compilation target.
type Target struct {
	OS   string // "linux", "darwin", "windows"
	Arch string // "amd64", "arm64"
}

// String returns the GOOS/GOARCH string.
func (t Target) String() string {
	return t.OS + "/" + t.Arch
}

// BinaryName returns the output binary name for this target.
func (t Target) BinaryName() string {
	name := fmt.Sprintf("trasker-client-%s-%s", t.OS, t.Arch)
	if t.OS == "windows" {
		name += ".exe"
	}
	return name
}

// SupportedTargets lists all valid build targets.
var SupportedTargets = []Target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

// ValidateTarget checks if a target is in the supported list.
func ValidateTarget(os, arch string) (Target, error) {
	for _, t := range SupportedTargets {
		if t.OS == os && t.Arch == arch {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unsupported target: %s/%s", os, arch)
}

// BuildRequest contains all information needed to build a client binary.
type BuildRequest struct {
	Target    Target
	APIKey    string // Plaintext API key to bake in
	ServerURL string // Server URL to bake in
	Version   string // Version string to embed
	OutputDir string // Directory to write the binary
}

// BuildResult contains the result of a build.
type BuildResult struct {
	BinaryPath string
	Target     Target
	Err        error
}

// Builder cross-compiles client binaries with stamped configuration.
type Builder struct {
	// SourceDir is the path to the Go module root (where go.mod lives).
	SourceDir string

	// ClientPkg is the import path of the client main package,
	// relative to the module root.
	ClientPkg string

	mu       sync.Mutex
	building map[string]bool // key: target string, value: in progress
}

// NewBuilder creates a Builder.
//
// sourceDir: absolute path to the Go module root.
// clientPkg: relative import path, e.g. "./cmd/trasker-client".
func NewBuilder(sourceDir, clientPkg string) *Builder {
	return &Builder{
		SourceDir: sourceDir,
		ClientPkg: clientPkg,
		building:  make(map[string]bool),
	}
}

// IsBuilding reports whether a build is in progress for the given target.
func (b *Builder) IsBuilding(t Target) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.building[t.String()]
}

// Build cross-compiles the client binary for the given request.
//
// It sets GOOS/GOARCH and uses -ldflags to stamp the API key, server URL,
// and version into the binary. Since trasker uses modernc.org/sqlite (pure Go),
// CGO_ENABLED=0 works for all targets — no C cross-compilers needed.
//
// NOTE: This invokes `go build` directly, so the Go toolchain must be available.
// In Docker deployment, the server API container does NOT include Go.
// The builder runs inside the separate Dockerfile.builder container.
// The server API invokes builds via `docker exec` or a build queue:
//
//	docker exec trasker-builder /build.sh --goos=linux --goarch=amd64 --api-key=... --server-url=...
//
// The Build() method below is used when Go is available locally (dev mode).
// For production Docker deployment, use BuildViaDocker() which shells out to the builder container.
func (b *Builder) Build(ctx context.Context, req BuildRequest) BuildResult {
	targetKey := req.Target.String()

	b.mu.Lock()
	if b.building[targetKey] {
		b.mu.Unlock()
		return BuildResult{
			Target: req.Target,
			Err:    fmt.Errorf("build already in progress for %s", targetKey),
		}
	}
	b.building[targetKey] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.building, targetKey)
		b.mu.Unlock()
	}()

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return BuildResult{Target: req.Target, Err: fmt.Errorf("create output dir: %w", err)}
	}

	outputPath := filepath.Join(req.OutputDir, req.Target.BinaryName())

	// Build ldflags to stamp values into the binary.
	// These correspond to variables in cmd/trasker-client/main.go:
	//   var apiKey string
	//   var serverURL string
	//   var version string
	ldflags := fmt.Sprintf(
		"-s -w -X main.apiKey=%s -X main.serverURL=%s -X main.version=%s",
		req.APIKey, req.ServerURL, req.Version,
	)

	args := []string{
		"build",
		"-ldflags", ldflags,
		"-trimpath",
		"-o", outputPath,
		req.Target.BinaryName(), // throwaway — overridden by -o
	}
	// The actual package to build
	args[len(args)-1] = b.ClientPkg

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = b.SourceDir
	cmd.Env = append(os.Environ(),
		"GOOS="+req.Target.OS,
		"GOARCH="+req.Target.Arch,
		"CGO_ENABLED=0",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return BuildResult{
			Target: req.Target,
			Err:    fmt.Errorf("go build failed: %w\noutput: %s", err, string(output)),
		}
	}

	return BuildResult{
		BinaryPath: outputPath,
		Target:     req.Target,
	}
}
