package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// mockAPIKeyStore implements auth.APIKeyLookup for testing.
type mockAPIKeyStore struct {
	keys map[string]*auth.APIKeyRecord // keyed by hash-matchable plaintext
}

func (m *mockAPIKeyStore) ListAllActiveAPIKeys(ctx context.Context) ([]auth.APIKeyRecord, error) {
	var result []auth.APIKeyRecord
	for _, k := range m.keys {
		result = append(result, *k)
	}
	return result, nil
}

func (m *mockAPIKeyStore) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	return nil
}

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	plainKey := "trsk_abcdef1234567890abcdef1234567890"
	hash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	require.NoError(t, err)

	userID := uuid.New()
	keyID := uuid.New()

	store := &mockAPIKeyStore{
		keys: map[string]*auth.APIKeyRecord{
			plainKey: {
				ID:        keyID,
				UserID:    userID,
				KeyHash:   string(hash),
				Role:      "member",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				Revoked:   false,
			},
		},
	}

	var capturedUserID uuid.UUID
	var capturedAuthType auth.AuthType
	handler := auth.APIKeyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID, _ = auth.UserIDFrom(r.Context())
		capturedAuthType, _ = auth.AuthTypeFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/timesheets", nil)
	req.Header.Set("Authorization", "Bearer "+plainKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, capturedUserID)
	assert.Equal(t, auth.AuthTypeAPIKey, capturedAuthType)
}

func TestAPIKeyMiddleware_MissingHeader(t *testing.T) {
	store := &mockAPIKeyStore{keys: map[string]*auth.APIKeyRecord{}}

	handler := auth.APIKeyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/timesheets", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	store := &mockAPIKeyStore{keys: map[string]*auth.APIKeyRecord{}}

	handler := auth.APIKeyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/timesheets", nil)
	req.Header.Set("Authorization", "Bearer trsk_invalidkey")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAPIKeyMiddleware_ExpiredKey(t *testing.T) {
	plainKey := "trsk_expired1234567890abcdef12345"
	hash, err := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	require.NoError(t, err)

	store := &mockAPIKeyStore{
		keys: map[string]*auth.APIKeyRecord{
			plainKey: {
				ID:        uuid.New(),
				UserID:    uuid.New(),
				KeyHash:   string(hash),
				Role:      "member",
				ExpiresAt: time.Now().Add(-1 * time.Hour), // expired
				Revoked:   false,
			},
		},
	}

	handler := auth.APIKeyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api/v1/timesheets", nil)
	req.Header.Set("Authorization", "Bearer "+plainKey)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
