package builder

import (
	"testing"
)

func TestValidateTarget_Supported(t *testing.T) {
	target, err := ValidateTarget("linux", "amd64")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if target.OS != "linux" || target.Arch != "amd64" {
		t.Fatalf("expected linux/amd64, got: %s", target.String())
	}
}

func TestValidateTarget_Unsupported(t *testing.T) {
	_, err := ValidateTarget("plan9", "mips")
	if err == nil {
		t.Fatal("expected error for unsupported target, got nil")
	}
}

func TestTargetBinaryName_Linux(t *testing.T) {
	target := Target{OS: "linux", Arch: "amd64"}
	expected := "trasker-client-linux-amd64"
	if got := target.BinaryName(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestTargetBinaryName_Windows(t *testing.T) {
	target := Target{OS: "windows", Arch: "amd64"}
	expected := "trasker-client-windows-amd64.exe"
	if got := target.BinaryName(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder("/path/to/src", "./cmd/trasker-client")
	if b.SourceDir != "/path/to/src" {
		t.Fatalf("expected /path/to/src, got %s", b.SourceDir)
	}
	if b.ClientPkg != "./cmd/trasker-client" {
		t.Fatalf("expected ./cmd/trasker-client, got %s", b.ClientPkg)
	}
}

func TestIsBuilding_DefaultFalse(t *testing.T) {
	b := NewBuilder("/tmp", "./cmd/trasker-client")
	target := Target{OS: "linux", Arch: "amd64"}
	if b.IsBuilding(target) {
		t.Fatal("expected IsBuilding to return false for new builder")
	}
}
