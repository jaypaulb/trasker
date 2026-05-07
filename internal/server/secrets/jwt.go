// Package secrets provides loaders for sensitive runtime values.
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateJWTSecret returns a JWT secret using this precedence:
//  1. Environment variable (if envKey is non-empty and the env var is set)
//  2. Contents of filePath (if file exists and is non-empty)
//  3. Generate 32 random bytes, hex-encode, write to filePath, return.
//
// Pass empty envKey to skip step 1 entirely.
func LoadOrGenerateJWTSecret(envKey, filePath string) (string, error) {
	// 1. Env var wins
	if envKey != "" {
		if val := os.Getenv(envKey); val != "" {
			return val, nil
		}
	}

	// 2. Existing file
	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		return string(data), nil
	}

	// 3. Generate new
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating random secret: %w", err)
	}
	secret := hex.EncodeToString(buf)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		return "", fmt.Errorf("creating secret directory: %w", err)
	}

	if err := os.WriteFile(filePath, []byte(secret), 0600); err != nil {
		return "", fmt.Errorf("writing secret file: %w", err)
	}

	return secret, nil
}
