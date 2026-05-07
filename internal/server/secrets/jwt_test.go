package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerate_EnvVar(t *testing.T) {
	t.Setenv("TEST_JWT_SECRET", "env-secret-value-that-is-at-least-32-bytes!")
	secret, err := LoadOrGenerateJWTSecret("TEST_JWT_SECRET", filepath.Join(t.TempDir(), "jwt-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if secret != "env-secret-value-that-is-at-least-32-bytes!" {
		t.Fatalf("expected env value, got %q", secret)
	}
}

func TestLoadOrGenerate_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")
	if err := os.WriteFile(path, []byte("file-secret-value"), 0600); err != nil {
		t.Fatal(err)
	}

	secret, err := LoadOrGenerateJWTSecret("NONEXISTENT_VAR", path)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "file-secret-value" {
		t.Fatalf("expected file value, got %q", secret)
	}
}

func TestLoadOrGenerate_GeneratesNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")

	secret, err := LoadOrGenerateJWTSecret("NONEXISTENT_VAR", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64-char hex string, got len=%d", len(secret))
	}

	// File should exist now
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("file not created:", err)
	}
	if string(data) != secret {
		t.Fatal("file content doesn't match returned secret")
	}
}

func TestLoadOrGenerate_EnvTakesPrecedenceOverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-secret")
	if err := os.WriteFile(path, []byte("file-value"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEST_JWT_PREC", "env-wins")
	secret, err := LoadOrGenerateJWTSecret("TEST_JWT_PREC", path)
	if err != nil {
		t.Fatal(err)
	}
	if secret != "env-wins" {
		t.Fatalf("expected env to win, got %q", secret)
	}
}
