package apikey

import (
	"strings"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	plaintext, hash, prefix, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	if plaintext == "" {
		t.Error("plaintext is empty")
	}
	if !strings.HasPrefix(plaintext, "tsk_") {
		t.Errorf("plaintext should start with 'tsk_', got %q", plaintext[:10])
	}

	if hash == "" {
		t.Error("hash is empty")
	}
	if !strings.HasPrefix(hash, "$2a$") {
		t.Errorf("hash should be bcrypt, got prefix %q", hash[:4])
	}

	if len(prefix) != 8 {
		t.Errorf("prefix length: got %d, want 8", len(prefix))
	}
	if prefix != plaintext[:8] {
		t.Errorf("prefix should be first 8 chars of plaintext: got %q, want %q", prefix, plaintext[:8])
	}
}

func TestGenerateUniqueness(t *testing.T) {
	p1, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() #1 error: %v", err)
	}
	p2, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() #2 error: %v", err)
	}
	if p1 == p2 {
		t.Error("two generated keys should not be identical")
	}
}

func TestHashAndVerify(t *testing.T) {
	plaintext, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	hash, err := Hash(plaintext)
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	if !Verify(plaintext, hash) {
		t.Error("Verify() should return true for matching key and hash")
	}
}

func TestVerifyWrongKey(t *testing.T) {
	p1, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	p2, _, _, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	hash, err := Hash(p1)
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	if Verify(p2, hash) {
		t.Error("Verify() should return false for non-matching key")
	}
}

func TestVerifyEmptyInputs(t *testing.T) {
	if Verify("", "somehash") {
		t.Error("Verify() should return false for empty key")
	}
	if Verify("somekey", "") {
		t.Error("Verify() should return false for empty hash")
	}
}

func TestPrefix(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "normal key", key: "tsk_abcdefghijklmnop", want: "tsk_abcd"},
		{name: "exactly 8 chars", key: "12345678", want: "12345678"},
		{name: "short key", key: "abc", want: "abc"},
		{name: "empty key", key: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Prefix(tt.key)
			if got != tt.want {
				t.Errorf("Prefix(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name       string
		lastUsed   time.Time
		expiryDays int
		want       bool
	}{
		{
			name:       "used yesterday, 60 day expiry",
			lastUsed:   time.Now().Add(-24 * time.Hour),
			expiryDays: 60,
			want:       false,
		},
		{
			name:       "used 61 days ago, 60 day expiry",
			lastUsed:   time.Now().Add(-61 * 24 * time.Hour),
			expiryDays: 60,
			want:       true,
		},
		{
			name:       "used exactly 60 days ago, 60 day expiry",
			lastUsed:   time.Now().Add(-60 * 24 * time.Hour),
			expiryDays: 60,
			want:       false,
		},
		{
			name:       "used just now, 1 day expiry",
			lastUsed:   time.Now(),
			expiryDays: 1,
			want:       false,
		},
		{
			name:       "zero expiry days means never expires",
			lastUsed:   time.Now().Add(-365 * 24 * time.Hour),
			expiryDays: 0,
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExpired(tt.lastUsed, tt.expiryDays)
			if got != tt.want {
				t.Errorf("IsExpired(%v, %d) = %v, want %v", tt.lastUsed, tt.expiryDays, got, tt.want)
			}
		})
	}
}
