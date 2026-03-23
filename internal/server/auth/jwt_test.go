package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWT_IssueAndValidate(t *testing.T) {
	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	userID := uuid.New()
	token, err := issuer.Issue(userID, "alice@example.com", "admin")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := issuer.Validate(token)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "alice@example.com", claims.Email)
	assert.Equal(t, "admin", claims.Role)
}

func TestJWT_ExpiredToken(t *testing.T) {
	secret := "test-secret-32-bytes-minimum!!!!!"
	// Issue with 0 duration so it expires immediately
	issuer, err := auth.NewJWTIssuer(secret, -1*time.Second)
	require.NoError(t, err)

	userID := uuid.New()
	token, err := issuer.Issue(userID, "bob@example.com", "member")
	require.NoError(t, err)

	_, err = issuer.Validate(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	userID := uuid.New()
	token, err := issuer.Issue(userID, "alice@example.com", "admin")
	require.NoError(t, err)

	var capturedUserID uuid.UUID
	var capturedRole string
	handler := auth.JWTMiddleware(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID, _ = auth.UserIDFrom(r.Context())
		capturedRole, _ = auth.UserRoleFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, capturedUserID)
	assert.Equal(t, "admin", capturedRole)
}

func TestJWTMiddleware_MissingToken(t *testing.T) {
	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	handler := auth.JWTMiddleware(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
