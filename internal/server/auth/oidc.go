package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCParams holds the configuration for Entra OIDC.
type OIDCParams struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OIDCConfig holds the initialized OIDC provider and OAuth2 config.
type OIDCConfig struct {
	Provider     *oidc.Provider
	OAuth2Config oauth2.Config
	Verifier     *oidc.IDTokenVerifier
}

// NewOIDCConfig validates parameters and prepares the OIDC configuration.
// The actual provider initialization (which contacts the Entra discovery endpoint)
// is done via InitProvider, which requires network access.
func NewOIDCConfig(p OIDCParams) (*OIDCConfig, error) {
	if p.TenantID == "" {
		return nil, fmt.Errorf("oidc: tenant_id is required")
	}
	if p.ClientID == "" {
		return nil, fmt.Errorf("oidc: client_id is required")
	}
	if p.RedirectURL == "" {
		return nil, fmt.Errorf("oidc: redirect_url is required")
	}

	return &OIDCConfig{
		OAuth2Config: oauth2.Config{
			ClientID:     p.ClientID,
			ClientSecret: p.ClientSecret,
			RedirectURL:  p.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, nil
}

// InitProvider contacts the Entra discovery endpoint and initializes the OIDC provider.
// Call this at server startup (requires network).
func (c *OIDCConfig) InitProvider(ctx context.Context, tenantID string) error {
	issuerURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", tenantID)

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return fmt.Errorf("oidc: initializing provider for tenant %s: %w", tenantID, err)
	}

	c.Provider = provider
	c.OAuth2Config.Endpoint = provider.Endpoint()
	c.Verifier = provider.Verifier(&oidc.Config{ClientID: c.OAuth2Config.ClientID})

	return nil
}

// UserInfo holds the extracted user information from OIDC claims.
type UserInfo struct {
	EntraOID    string
	Email       string
	DisplayName string
}

// ExtractUserInfo extracts user information from OIDC token claims.
func ExtractUserInfo(claims map[string]any) (*UserInfo, error) {
	oid, ok := claims["oid"].(string)
	if !ok || oid == "" {
		return nil, fmt.Errorf("oidc claims: missing or empty 'oid' field")
	}

	email, _ := claims["preferred_username"].(string)
	if email == "" {
		email, _ = claims["email"].(string)
	}
	if email == "" {
		return nil, fmt.Errorf("oidc claims: missing email (checked preferred_username and email)")
	}

	name, _ := claims["name"].(string)
	if name == "" {
		name = email // fallback to email as display name
	}

	return &UserInfo{
		EntraOID:    oid,
		Email:       email,
		DisplayName: name,
	}, nil
}
