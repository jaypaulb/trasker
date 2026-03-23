// internal/server/api/admin_handlers_test.go
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
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func setupAdminTestServer(t *testing.T) (http.Handler, *store.Store, *store.User, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	admin, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "admin-handler-oid", Email: "adminhandler@example.com", DisplayName: "Admin Handler",
	})
	require.NoError(t, err)
	_, err = s.UpdateUserRole(context.Background(), admin.ID, "admin")
	require.NoError(t, err)

	token, err := issuer.Issue(admin.ID, admin.Email, "admin")
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, JWTIssuer: issuer}
	router := api.NewRouter(deps)

	return router, s, admin, token
}

func TestAdminHandler_DeleteEntry(t *testing.T) {
	router, s, admin, token := setupAdminTestServer(t)

	ctx := context.Background()
	plainKey := "trsk_admindeletetest12345678901"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		UserID: admin.ID, KeyHash: string(hash), KeyPrefix: "trsk_adl", ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)
	device, err := s.UpsertDevice(ctx, store.UpsertDeviceParams{
		UserID: admin.ID, APIKeyID: apiKey.ID, ClientDeviceID: "admin-device", OS: "linux",
	})
	require.NoError(t, err)

	now := time.Now()
	ts, err := s.CreateTimesheet(ctx, store.CreateTimesheetParams{
		UserID: admin.ID, DeviceID: device.ID, SubmittedAt: now,
		Entries: []store.CreateTimesheetEntryParams{
			{Tag: "Dev", StartedAt: now.Add(-1 * time.Hour), EndedAt: now, DurationS: 3600},
		},
	})
	require.NoError(t, err)

	entryID := ts.Entries[0].ID.String()
	req := httptest.NewRequest("DELETE", "/api/v1/admin/entries/"+entryID, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestAdminHandler_GetSettings(t *testing.T) {
	router, s, _, token := setupAdminTestServer(t)

	// Create settings first
	_, err := s.UpsertOrgSettings(context.Background(), store.UpdateOrgSettingsParams{
		OrgName:     strPtrAPI("Test Org"),
		EntraTenant: strPtrAPI("tenant-123"),
		EntraClient: strPtrAPI("client-456"),
	})
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/admin/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "Test Org", resp["org_name"])
}

func TestAdminHandler_UpdateSettings(t *testing.T) {
	router, s, _, token := setupAdminTestServer(t)

	// Seed settings
	_, err := s.UpsertOrgSettings(context.Background(), store.UpdateOrgSettingsParams{
		OrgName:     strPtrAPI("Old Org"),
		EntraTenant: strPtrAPI("tenant-old"),
		EntraClient: strPtrAPI("client-old"),
	})
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]any{
		"org_name":        "New Org",
		"key_expiry_days": 90,
	})
	req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "New Org", resp["org_name"])
	assert.Equal(t, float64(90), resp["key_expiry_days"])
}

func TestAdminHandler_AuditLog(t *testing.T) {
	router, _, _, token := setupAdminTestServer(t)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func strPtrAPI(s string) *string { return &s }
