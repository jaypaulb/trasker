package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// setupDeviceTestServer creates a real store + router for device handler tests.
// Requires testcontainers (integration test).
func setupDeviceTestServer(t *testing.T) (*api.Dependencies, http.Handler, *store.User, *store.APIKey, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	user, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "dev-handler-oid", Email: "devhandler@example.com", DisplayName: "Dev Handler",
	})
	require.NoError(t, err)

	plainKey := "tsk_devicehandlertest1234567890"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   string(hash),
		KeyPrefix: plainKey[:8],
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, APIKeyAuth: api.NewStoreAPIKeyAdapter(s)}
	router := api.NewRouter(deps)

	return deps, router, user, apiKey, plainKey
}

func TestDeviceHandler_Register(t *testing.T) {
	_, router, _, _, plainKey := setupDeviceTestServer(t)

	body, _ := json.Marshal(map[string]string{
		"client_device_id": "device-uuid-100",
		"os":               "linux",
	})

	req := httptest.NewRequest("POST", "/api/v1/devices", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plainKey)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "device-uuid-100", resp["client_device_id"])
}

func TestDeviceHandler_List(t *testing.T) {
	_, router, _, _, plainKey := setupDeviceTestServer(t)

	// Register a device first
	body, _ := json.Marshal(map[string]string{
		"client_device_id": "device-uuid-200",
		"os":               "darwin",
	})
	req := httptest.NewRequest("POST", "/api/v1/devices", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	// List devices
	req2 := httptest.NewRequest("GET", "/api/v1/devices", nil)
	req2.Header.Set("Authorization", "Bearer "+plainKey)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusOK, rr2.Code)

	var resp []map[string]any
	err := json.Unmarshal(rr2.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp), 1)
}
