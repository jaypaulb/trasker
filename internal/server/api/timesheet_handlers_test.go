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

func TestTimesheetHandler_Submit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	user, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "ts-handler-oid", Email: "tshandler@example.com", DisplayName: "TS Handler",
	})
	require.NoError(t, err)

	plainKey := "trsk_timesheethandlertest123456789"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID: user.ID, KeyHash: string(hash), KeyPrefix: "trsk_tsh", ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)

	// Register a device first (needed for timesheet FK)
	_, err = s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID: user.ID, APIKeyID: apiKey.ID, ClientDeviceID: "ts-device-1", OS: "linux",
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, APIKeyAuth: api.NewStoreAPIKeyAdapter(s)}
	router := api.NewRouter(deps)

	now := time.Now().UTC()
	body, _ := json.Marshal(map[string]any{
		"client_device_id": "ts-device-1",
		"entries": []map[string]any{
			{
				"tag":        "Development",
				"started_at": now.Add(-3 * time.Hour).Format(time.RFC3339),
				"ended_at":   now.Format(time.RFC3339),
				"duration_s": 10800,
				"notes":      "Working on auth",
			},
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/timesheets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plainKey)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["id"])
}

func TestTimesheetHandler_ListOwn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	user, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "ts-list-oid", Email: "tslist@example.com", DisplayName: "TS List",
	})
	require.NoError(t, err)

	plainKey := "trsk_timesheetlisttest1234567890"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID: user.ID, KeyHash: string(hash), KeyPrefix: "trsk_tsl", ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)

	device, err := s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID: user.ID, APIKeyID: apiKey.ID, ClientDeviceID: "ts-list-device", OS: "linux",
	})
	require.NoError(t, err)

	// Create a timesheet directly
	now := time.Now()
	_, err = s.CreateTimesheet(context.Background(), store.CreateTimesheetParams{
		UserID: user.ID, DeviceID: device.ID, SubmittedAt: now,
		Entries: []store.CreateTimesheetEntryParams{
			{Tag: "Dev", StartedAt: now.Add(-1 * time.Hour), EndedAt: now, DurationS: 3600},
		},
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, APIKeyAuth: api.NewStoreAPIKeyAdapter(s)}
	router := api.NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/v1/timesheets", nil)
	req.Header.Set("Authorization", "Bearer "+plainKey)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
