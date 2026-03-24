// Package apikey handles API key generation, hashing, verification, and expiry
// checks. Keys are generated with a "tsk_" prefix for easy identification.
// Server stores bcrypt hashes; plaintext only exists in client binaries.
package apikey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// keyPrefix is prepended to all generated API keys for identification.
	keyPrefix = "tsk_"

	// randomBytes is the number of random bytes used to generate the key body.
	// 32 bytes = 64 hex chars, giving 256 bits of entropy.
	randomBytes = 32

	// bcryptCost is the bcrypt work factor. 12 is a reasonable default for
	// API key hashing (not user-facing latency-sensitive).
	bcryptCost = 12

	// prefixLen is the number of characters stored as the key prefix for
	// identification in the admin UI.
	prefixLen = 8
)

// Generate creates a new API key and returns the plaintext key, its bcrypt
// hash, and the prefix (first 8 characters) for storage.
func Generate() (plaintext, hash, prefix string, err error) {
	b := make([]byte, randomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}

	plaintext = keyPrefix + hex.EncodeToString(b)

	hash, err = Hash(plaintext)
	if err != nil {
		return "", "", "", err
	}

	prefix = Prefix(plaintext)
	return plaintext, hash, prefix, nil
}

// Hash returns the bcrypt hash of the given API key.
func Hash(key string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(key), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(h), nil
}

// Verify checks whether the given plaintext key matches the bcrypt hash.
// Returns false for empty inputs or mismatches.
func Verify(key, hash string) bool {
	if key == "" || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)) == nil
}

// Prefix returns the first 8 characters of the key for identification.
// If the key is shorter than 8 characters, the full key is returned.
func Prefix(key string) string {
	if len(key) <= prefixLen {
		return key
	}
	return key[:prefixLen]
}

// IsExpired returns true if the key has not been used within expiryDays
// of the current time. If expiryDays is 0, the key never expires.
// A key used exactly expiryDays ago is NOT considered expired; only keys
// used strictly more than expiryDays days ago are expired.
func IsExpired(lastUsed time.Time, expiryDays int) bool {
	if expiryDays <= 0 {
		return false
	}
	// Truncate both times to the day boundary so that "exactly N days ago"
	// is not considered expired. This avoids sub-second races between the
	// caller computing lastUsed and IsExpired calling time.Now().
	now := time.Now().Truncate(24 * time.Hour)
	last := lastUsed.Truncate(24 * time.Hour)
	elapsed := int(now.Sub(last).Hours() / 24)
	return elapsed > expiryDays
}
