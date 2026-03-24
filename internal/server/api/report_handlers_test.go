// internal/server/api/report_handlers_test.go
package api_test

import (
	"context"
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

func TestReportHandler_Summary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	manager, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "report-mgr-oid", Email: "reportmgr@example.com", DisplayName: "Report Manager",
	})
	require.NoError(t, err)
	_, err = s.UpdateUserRole(context.Background(), manager.ID, "manager")
	require.NoError(t, err)

	// Create test data: user with timesheet
	plainKey := "tsk_reporttestkey12345678901234"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID: manager.ID, KeyHash: string(hash), KeyPrefix: plainKey[:8], ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)
	device, err := s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID: manager.ID, APIKeyID: apiKey.ID, ClientDeviceID: "report-device", OS: "linux",
	})
	require.NoError(t, err)

	now := time.Now()
	_, err = s.CreateTimesheet(context.Background(), store.CreateTimesheetParams{
		UserID: manager.ID, DeviceID: device.ID, SubmittedAt: now,
		Entries: []store.CreateTimesheetEntryParams{
			{Tag: "Dev", StartedAt: now.Add(-3 * time.Hour), EndedAt: now.Add(-1 * time.Hour), DurationS: 7200},
			{Tag: "Meeting", StartedAt: now.Add(-1 * time.Hour), EndedAt: now, DurationS: 3600},
		},
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, JWTIssuer: issuer}
	router := api.NewRouter(deps)

	token, err := issuer.Issue(manager.ID, manager.Email, "manager")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/reports/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestReportHandler_Export_CSV(t *testing.T) {
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
		EntraOID: "export-admin-oid", Email: "exportadmin@example.com", DisplayName: "Export Admin",
	})
	require.NoError(t, err)
	_, err = s.UpdateUserRole(context.Background(), admin.ID, "admin")
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, JWTIssuer: issuer}
	router := api.NewRouter(deps)

	token, err := issuer.Issue(admin.ID, admin.Email, "admin")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/reports/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/csv", rr.Header().Get("Content-Type"))
}
