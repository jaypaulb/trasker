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

func TestSentinelLengths(t *testing.T) {
	if len(SentinelServerURL) != SentinelLen {
		t.Fatalf("SentinelServerURL is %d bytes, expected %d", len(SentinelServerURL), SentinelLen)
	}
	if len(SentinelAPIKey) != SentinelLen {
		t.Fatalf("SentinelAPIKey is %d bytes, expected %d", len(SentinelAPIKey), SentinelLen)
	}
	if len(SentinelVersion) != SentinelLen {
		t.Fatalf("SentinelVersion is %d bytes, expected %d", len(SentinelVersion), SentinelLen)
	}
}

func TestPadToLen(t *testing.T) {
	result := padToLen("hello", 10)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 10 {
		t.Fatalf("expected length 10, got %d", len(result))
	}
	if string(result[:5]) != "hello" {
		t.Fatalf("expected 'hello' prefix, got %q", string(result[:5]))
	}
	// Remaining bytes should be null
	for i := 5; i < 10; i++ {
		if result[i] != 0 {
			t.Fatalf("expected null byte at index %d, got %d", i, result[i])
		}
	}
}

func TestPadToLen_TooLong(t *testing.T) {
	result := padToLen("hello", 3)
	if result != nil {
		t.Fatal("expected nil for string longer than target length")
	}
}

func TestIsCached_DefaultFalse(t *testing.T) {
	b := NewBuilder("/tmp", "./cmd/trasker-client")
	target := Target{OS: "linux", Arch: "amd64"}
	if b.IsCached(target) {
		t.Fatal("expected IsCached to return false for new builder")
	}
}
