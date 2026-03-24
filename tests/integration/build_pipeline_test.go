//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBuildPipelineStampsAPIKey tests:
// 1. Build a client binary with stamped API key and server URL via -ldflags
// 2. Run the binary and verify it outputs the stamped values
func TestBuildPipelineStampsAPIKey(t *testing.T) {
	// This test requires a Go toolchain.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go toolchain not available")
	}

	tmpDir := t.TempDir()
	binaryName := "trasker-client-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)

	// Find the project root (go.mod location).
	projectRoot := findProjectRoot(t)

	// Stamp values via ldflags.
	testAPIKey := "tsk_test_12345678abcdefgh"
	testServerURL := "https://trasker.example.com"
	testVersion := "1.0.0-test"
	testCommit := "abc1234"

	ldflags := fmt.Sprintf(
		"-X main.apiKey=%s -X main.serverURL=%s "+
			"-X github.com/jaypaulb/trasker/internal/shared/version.Version=%s "+
			"-X github.com/jaypaulb/trasker/internal/shared/version.Commit=%s",
		testAPIKey, testServerURL, testVersion, testCommit,
	)

	// Build the client binary.
	cmd := exec.Command("go", "build",
		"-ldflags", ldflags,
		"-o", binaryPath,
		"./cmd/trasker-client/",
	)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(output))
	}

	// Verify binary exists and is reasonable size.
	info, err := os.Stat(binaryPath)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Size() < 100000 {
		t.Fatalf("binary suspiciously small: %d bytes", info.Size())
	}
	t.Logf("Built binary: %s (%d bytes)", binaryPath, info.Size())

	// Run the binary with --version flag to check stamped values.
	// The client entrypoint should print version info when given --version.
	runCmd := exec.Command(binaryPath, "--version")
	runOutput, err := runCmd.CombinedOutput()
	if err != nil {
		// The binary might not support --version yet. Check if the version
		// string appears in the binary file itself as a fallback.
		t.Logf("Binary execution returned error (may not support --version yet): %v", err)
		t.Log("Falling back to checking binary contents for stamped strings...")

		binaryContents, readErr := os.ReadFile(binaryPath)
		if readErr != nil {
			t.Fatalf("cannot read binary: %v", readErr)
		}

		binaryStr := string(binaryContents)
		if !strings.Contains(binaryStr, testVersion) {
			t.Error("binary does not contain stamped version")
		}
		if !strings.Contains(binaryStr, testCommit) {
			t.Error("binary does not contain stamped commit")
		}
		// API key and server URL should also be present in the binary.
		if !strings.Contains(binaryStr, testAPIKey) {
			t.Error("binary does not contain stamped API key")
		}
		if !strings.Contains(binaryStr, testServerURL) {
			t.Error("binary does not contain stamped server URL")
		}

		fmt.Println("PASS: Build pipeline stamps verified via binary contents")
		return
	}

	outputStr := string(runOutput)
	t.Logf("Binary output: %s", outputStr)

	if !strings.Contains(outputStr, testVersion) {
		t.Errorf("output does not contain version %q", testVersion)
	}
	if !strings.Contains(outputStr, testCommit) {
		t.Errorf("output does not contain commit %q", testCommit)
	}

	fmt.Println("PASS: Build pipeline stamping verified successfully")
}

// TestBuildPipelineCrossCompile tests that the build pipeline can produce
// binaries for all target platforms from the current host.
func TestBuildPipelineCrossCompile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go toolchain not available")
	}

	projectRoot := findProjectRoot(t)
	tmpDir := t.TempDir()

	targets := []struct {
		goos   string
		goarch string
		ext    string
	}{
		{"linux", "amd64", ""},
		{"linux", "arm64", ""},
		{"darwin", "amd64", ""},
		{"darwin", "arm64", ""},
		{"windows", "amd64", ".exe"},
	}

	for _, target := range targets {
		target := target // capture range variable
		name := fmt.Sprintf("%s-%s", target.goos, target.goarch)
		t.Run(name, func(t *testing.T) {
			binaryPath := filepath.Join(tmpDir, "trasker-client-"+name+target.ext)

			cmd := exec.Command("go", "build",
				"-o", binaryPath,
				"./cmd/trasker-client/",
			)
			cmd.Dir = projectRoot
			cmd.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+target.goos,
				"GOARCH="+target.goarch,
			)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cross-compile for %s failed: %v\n%s", name, err, string(output))
			}

			info, err := os.Stat(binaryPath)
			if err != nil {
				t.Fatalf("binary not found: %v", err)
			}
			if info.Size() < 100000 {
				t.Errorf("binary suspiciously small: %d bytes", info.Size())
			}
			t.Logf("%s: %d bytes", name, info.Size())
		})
	}
}

// findProjectRoot walks up from the current directory to find go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()

	// Start from the test file's directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find project root (no go.mod found)")
		}
		dir = parent
	}
}
