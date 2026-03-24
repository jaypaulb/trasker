package auth_test

import (
	"testing"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCConfig_Validation(t *testing.T) {
	// Missing tenant
	_, err := auth.NewOIDCConfig(auth.OIDCParams{
		TenantID:    "",
		ClientID:    "client-123",
		RedirectURL: "https://example.com/callback",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id")

	// Missing client ID
	_, err = auth.NewOIDCConfig(auth.OIDCParams{
		TenantID:    "tenant-123",
		ClientID:    "",
		RedirectURL: "https://example.com/callback",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client_id")

	// Missing redirect URL
	_, err = auth.NewOIDCConfig(auth.OIDCParams{
		TenantID:    "tenant-123",
		ClientID:    "client-123",
		RedirectURL: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect_url")
}

func TestExtractUserInfo_FromClaims(t *testing.T) {
	claims := map[string]any{
		"oid":                "entra-oid-123",
		"preferred_username": "alice@example.com",
		"name":               "Alice Smith",
	}

	info, err := auth.ExtractUserInfo(claims)
	require.NoError(t, err)
	assert.Equal(t, "entra-oid-123", info.EntraOID)
	assert.Equal(t, "alice@example.com", info.Email)
	assert.Equal(t, "Alice Smith", info.DisplayName)
}

func TestExtractUserInfo_MissingOID(t *testing.T) {
	claims := map[string]any{
		"preferred_username": "alice@example.com",
		"name":               "Alice Smith",
	}

	_, err := auth.ExtractUserInfo(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oid")
}
