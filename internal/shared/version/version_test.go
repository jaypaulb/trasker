package version

import "testing"

func TestDefaultVersion(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected default Version to be %q, got %q", "dev", Version)
	}
}

func TestDefaultCommit(t *testing.T) {
	if Commit != "unknown" {
		t.Errorf("expected default Commit to be %q, got %q", "unknown", Commit)
	}
}

func TestDefaultAPIKey(t *testing.T) {
	if APIKey != "" {
		t.Errorf("expected default APIKey to be empty, got %q", APIKey)
	}
}

func TestDefaultServerURL(t *testing.T) {
	if ServerURL != "" {
		t.Errorf("expected default ServerURL to be empty, got %q", ServerURL)
	}
}

func TestString(t *testing.T) {
	want := "dev (unknown)"
	got := String()
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
