package builder

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Sentinel strings compiled into the generic client binary via -ldflags -X.
// At download time, the server does a bytes.Replace to swap these for real values.
// Each sentinel is exactly 128 bytes so the binary size never changes.
const (
	SentinelLen = 128

	// Prefix + padding to 128 chars. The prefix is unique enough to never appear naturally.
	SentinelServerURL = "TRASKER_SENTINEL_SERVER_URL_____" +
		"________________________________________________" +
		"________________________________________________"
	SentinelAPIKey = "TRASKER_SENTINEL_API_KEY________" +
		"________________________________________________" +
		"________________________________________________"
	SentinelVersion = "TRASKER_SENTINEL_VERSION________" +
		"________________________________________________" +
		"________________________________________________"
)

func init() {
	// Compile-time safety: ensure sentinels are exactly SentinelLen bytes.
	if len(SentinelServerURL) != SentinelLen {
		panic(fmt.Sprintf("SentinelServerURL is %d bytes, expected %d", len(SentinelServerURL), SentinelLen))
	}
	if len(SentinelAPIKey) != SentinelLen {
		panic(fmt.Sprintf("SentinelAPIKey is %d bytes, expected %d", len(SentinelAPIKey), SentinelLen))
	}
	if len(SentinelVersion) != SentinelLen {
		panic(fmt.Sprintf("SentinelVersion is %d bytes, expected %d", len(SentinelVersion), SentinelLen))
	}
}

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

// PatchRequest contains the values to patch into a cached binary.
type PatchRequest struct {
	Target    Target
	APIKey    string
	ServerURL string
	Version   string
}

// Builder cross-compiles client binaries and caches them for fast patching.
type Builder struct {
	SourceDir string // Go module root (where go.mod lives)
	ClientPkg string // e.g. "./cmd/trasker-client"

	mu    sync.RWMutex
	cache map[string][]byte // target string → generic binary bytes
}

// NewBuilder creates a Builder.
func NewBuilder(sourceDir, clientPkg string) *Builder {
	return &Builder{
		SourceDir: sourceDir,
		ClientPkg: clientPkg,
		cache:     make(map[string][]byte),
	}
}

// ClientBinDir is the default directory for pre-compiled client binaries
// baked into the container image by CI.
const ClientBinDir = "/app/clients"

// NewEmptyBuilder creates a Builder with no source directory (for loading
// pre-compiled binaries only — production deployments without a Go toolchain).
func NewEmptyBuilder() *Builder {
	return &Builder{
		cache: make(map[string][]byte),
	}
}

// LoadFromDir loads pre-compiled generic binaries from a directory on disk.
// Files must be named trasker-client-{os}-{arch}[.exe] and contain the API
// key sentinel (verifying they were built with the right ldflags). Returns
// the number of targets loaded. If the directory doesn't exist or is empty,
// returns 0 (not an error — caller should fall back to PreBuild or Build).
func (b *Builder) LoadFromDir(dir string, logger *slog.Logger) int {
	loaded := 0
	for _, target := range SupportedTargets {
		path := filepath.Join(dir, target.BinaryName())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Verify sentinel is present (catches binaries built without
		// the right ldflags — they'd be unpatchable at download time).
		if !bytes.Contains(data, []byte(SentinelAPIKey)) {
			logger.Warn("pre-compiled binary missing sentinel, skipping",
				"target", target.String(), "path", path)
			continue
		}

		b.mu.Lock()
		b.cache[target.String()] = data
		b.mu.Unlock()

		logger.Info("loaded pre-compiled client binary",
			"target", target.String(),
			"size_mb", fmt.Sprintf("%.1f", float64(len(data))/(1024*1024)),
		)
		loaded++
	}
	return loaded
}

// PreBuild compiles all supported targets with sentinel values and caches
// the resulting binaries in memory. Call this once at server startup (in a
// background goroutine — it takes 30-60s). Downloads are blocked until this
// completes for the requested target.
func (b *Builder) PreBuild(ctx context.Context, logger *slog.Logger) error {
	logger.Info("pre-building client binaries for all targets...")

	tmpDir, err := os.MkdirTemp("", "trasker-prebuild-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ldflags := fmt.Sprintf(
		"-s -w -X main.apiKey=%s -X main.serverURL=%s -X main.version=%s",
		SentinelAPIKey, SentinelServerURL, SentinelVersion,
	)

	for _, target := range SupportedTargets {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		outputPath := filepath.Join(tmpDir, target.BinaryName())
		args := []string{
			"build",
			"-ldflags", ldflags,
			"-trimpath",
			"-o", outputPath,
			b.ClientPkg,
		}

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = b.SourceDir
		cmd.Env = append(os.Environ(),
			"GOOS="+target.OS,
			"GOARCH="+target.Arch,
			"CGO_ENABLED=0",
		)

		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Error("pre-build failed", "target", target.String(), "error", err, "output", string(output))
			return fmt.Errorf("pre-build %s: %w\n%s", target.String(), err, output)
		}

		data, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("read built binary %s: %w", target.String(), err)
		}

		// Verify sentinels are present in the binary.
		if !bytes.Contains(data, []byte(SentinelAPIKey)) {
			return fmt.Errorf("sentinel API key not found in %s binary", target.String())
		}

		b.mu.Lock()
		b.cache[target.String()] = data
		b.mu.Unlock()

		logger.Info("pre-built client binary",
			"target", target.String(),
			"size_mb", fmt.Sprintf("%.1f", float64(len(data))/(1024*1024)),
		)
	}

	logger.Info("all client binaries pre-built and cached")
	return nil
}

// IsCached reports whether a pre-built binary is available for the target.
func (b *Builder) IsCached(t Target) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.cache[t.String()]
	return ok
}

// Patch takes a cached generic binary and replaces the sentinel strings with
// real values. Returns the patched binary bytes. This is O(n) over the binary
// size — typically ~50ms for a 15MB binary.
func (b *Builder) Patch(req PatchRequest) ([]byte, error) {
	b.mu.RLock()
	generic, ok := b.cache[req.Target.String()]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no cached binary for %s (pre-build may still be running)", req.Target.String())
	}

	// Pad each value to SentinelLen with null bytes.
	paddedURL := padToLen(req.ServerURL, SentinelLen)
	paddedKey := padToLen(req.APIKey, SentinelLen)
	paddedVer := padToLen(req.Version, SentinelLen)

	if paddedURL == nil || paddedKey == nil || paddedVer == nil {
		return nil, fmt.Errorf("value exceeds max length of %d bytes", SentinelLen)
	}

	// Copy to avoid mutating the cached slice.
	patched := make([]byte, len(generic))
	copy(patched, generic)

	patched = bytes.Replace(patched, []byte(SentinelServerURL), paddedURL, 1)
	patched = bytes.Replace(patched, []byte(SentinelAPIKey), paddedKey, 1)
	patched = bytes.Replace(patched, []byte(SentinelVersion), paddedVer, 1)

	return patched, nil
}

// padToLen pads s with null bytes to exactly length n.
// Returns nil if s is longer than n.
func padToLen(s string, n int) []byte {
	if len(s) > n {
		return nil
	}
	buf := make([]byte, n)
	copy(buf, s)
	// Remaining bytes are already zero (null padding).
	return buf
}

// --- Legacy Build method (kept for fallback / dev use) ---

// BuildRequest contains all information needed to build a client binary from scratch.
type BuildRequest struct {
	Target    Target
	APIKey    string
	ServerURL string
	Version   string
	OutputDir string
}

// BuildResult contains the result of a build.
type BuildResult struct {
	BinaryPath string
	Target     Target
	Err        error
}

// Build cross-compiles the client binary from scratch. This is the slow path
// (10-30s per target). Prefer PreBuild + Patch for production use.
func (b *Builder) Build(ctx context.Context, req BuildRequest) BuildResult {
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return BuildResult{Target: req.Target, Err: fmt.Errorf("create output dir: %w", err)}
	}

	outputPath := filepath.Join(req.OutputDir, req.Target.BinaryName())

	ldflags := fmt.Sprintf(
		"-s -w -X main.apiKey=%s -X main.serverURL=%s -X main.version=%s",
		req.APIKey, req.ServerURL, req.Version,
	)

	args := []string{
		"build",
		"-ldflags", ldflags,
		"-trimpath",
		"-o", outputPath,
		b.ClientPkg,
	}

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

// CacheStatus returns a summary of cached targets for status endpoints.
func (b *Builder) CacheStatus() map[string]bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	status := make(map[string]bool, len(SupportedTargets))
	for _, t := range SupportedTargets {
		_, ok := b.cache[t.String()]
		status[t.String()] = ok
	}
	return status
}

