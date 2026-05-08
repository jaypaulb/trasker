package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/shared/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// layoutTestEnv bundles the moving parts a handler test needs.
type layoutTestEnv struct {
	tdb      *testDB
	store    *store.Store
	router   http.Handler
	deps     *api.Dependencies
	user     *store.User
	device   *store.Device
	plainKey string // bearer for API-key auth
	jwt      string // bearer for JWT auth
}

// setupLayoutTestServer wires a real store + router for layout handler tests.
// It creates a user, an API key, a device, and a JWT bearer for the same user.
// Mirrors the device_handlers_test setup but adds a JWTIssuer so dashboard
// reads can be exercised against the JWT-protected group.
func setupLayoutTestServer(t *testing.T, suffix string) *layoutTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tdb := newTestDBForAPI(t)
	s, err := store.New(context.Background(), tdb.Pool)
	require.NoError(t, err)

	user, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID:    "lay-handler-oid-" + suffix,
		Email:       "lay" + suffix + "@example.com",
		DisplayName: "Layout " + suffix,
	})
	require.NoError(t, err)

	plainKey := "tsk_layouthandlerkey" + suffix + "0000000000"
	hash, _ := bcrypt.GenerateFromPassword([]byte(plainKey), bcrypt.DefaultCost)
	apiKey, err := s.CreateAPIKey(context.Background(), store.CreateAPIKeyParams{
		UserID:    user.ID,
		KeyHash:   string(hash),
		KeyPrefix: plainKey[:8],
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
	})
	require.NoError(t, err)

	device, err := s.UpsertDevice(context.Background(), store.UpsertDeviceParams{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		ClientDeviceID: "lay-device-" + suffix,
		OS:             "linux",
	})
	require.NoError(t, err)

	issuer, err := auth.NewJWTIssuer("layout-handler-test-secret-32bytes!", 15*time.Minute)
	require.NoError(t, err)
	jwtToken, err := issuer.Issue(user.ID, user.Email, models.Role(user.Role))
	require.NoError(t, err)

	deps := &api.Dependencies{
		Store:      s,
		APIKeyAuth: api.NewStoreAPIKeyAdapter(s),
		JWTIssuer:  issuer,
	}
	router := api.NewRouter(deps)

	return &layoutTestEnv{
		tdb:      tdb,
		store:    s,
		router:   router,
		deps:     deps,
		user:     user,
		device:   device,
		plainKey: plainKey,
		jwt:      jwtToken,
	}
}

// layoutWindow mirrors the inner Windows struct expected by the ingest handler.
type layoutWindow struct {
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
}

func ingestBody(deviceID string, captured time.Time, hash string, windows []layoutWindow) []byte {
	b, _ := json.Marshal(map[string]any{
		"client_device_id": deviceID,
		"snapshots": []map[string]any{
			{
				"captured_at":  captured.Format(time.RFC3339),
				"windows_hash": hash,
				"windows":      windows,
			},
		},
	})
	return b
}

func TestLayoutIngest_Single(t *testing.T) {
	env := setupLayoutTestServer(t, "aa")
	body := ingestBody(env.device.ClientDeviceID, time.Now().UTC(), "hash-single",
		[]layoutWindow{{AppName: "firefox", WindowTitle: "tab", X: 0, Y: 0, W: 800, H: 600}})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var count int
	err := env.tdb.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, env.device.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestLayoutIngest_Batch(t *testing.T) {
	env := setupLayoutTestServer(t, "bb")

	now := time.Now().UTC().Truncate(time.Second)
	snapshots := make([]map[string]any, 5)
	for i := 0; i < 5; i++ {
		snapshots[i] = map[string]any{
			"captured_at":  now.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			"windows_hash": fmt.Sprintf("hash-batch-%d", i),
			"windows": []layoutWindow{
				{AppName: "firefox", WindowTitle: "tab", X: 0, Y: 0, W: 800, H: 600},
			},
		}
	}
	body, _ := json.Marshal(map[string]any{
		"client_device_id": env.device.ClientDeviceID,
		"snapshots":        snapshots,
	})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var count int
	err := env.tdb.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, env.device.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestLayoutIngest_Dedup(t *testing.T) {
	env := setupLayoutTestServer(t, "cc")
	body := ingestBody(env.device.ClientDeviceID, time.Now().UTC().Truncate(time.Second),
		"hash-dedup",
		[]layoutWindow{{AppName: "firefox", WindowTitle: "t", X: 0, Y: 0, W: 1, H: 1}})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+env.plainKey)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)
		require.Equal(t, http.StatusCreated, rr.Code, "iteration %d body: %s", i, rr.Body.String())
	}

	var count int
	err := env.tdb.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM layout_snapshots WHERE device_id = $1`, env.device.ID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "second POST must dedup, not duplicate")
}

func TestLayoutIngest_BadAuth(t *testing.T) {
	env := setupLayoutTestServer(t, "dd")
	body := ingestBody(env.device.ClientDeviceID, time.Now().UTC(), "hash-bad",
		[]layoutWindow{{AppName: "x", WindowTitle: "y", X: 0, Y: 0, W: 1, H: 1}})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer not-a-real-key")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestLayoutIngest_BadDevice(t *testing.T) {
	env := setupLayoutTestServer(t, "ee")
	body := ingestBody("client-not-registered", time.Now().UTC(), "hash-baddev",
		[]layoutWindow{{AppName: "x", WindowTitle: "y", X: 0, Y: 0, W: 1, H: 1}})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "device not registered")
}

func TestLayoutIngest_OversizePayload(t *testing.T) {
	env := setupLayoutTestServer(t, "ff")
	// Build a body bigger than the 4MB layout cap by stuffing a long title.
	huge := strings.Repeat("A", 5*1024*1024)
	body := ingestBody(env.device.ClientDeviceID, time.Now().UTC(), "hash-huge",
		[]layoutWindow{{AppName: "firefox", WindowTitle: huge, X: 0, Y: 0, W: 1, H: 1}})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	// MaxBytesReader trips at JSON decode, surfacing as 400 or 413; either is
	// acceptable per the plan ("400 (or 413 from MaxBytesReader)").
	assert.Contains(t, []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge}, rr.Code,
		"got %d", rr.Code)
}

func TestLayoutIngest_InvalidJSON(t *testing.T) {
	env := setupLayoutTestServer(t, "gg")
	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots",
		bytes.NewReader([]byte("not-json-at-all")))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// TestLayoutIngest_PrivacyReject verifies the server-side strip — extra
// JSON fields like "pid" must NOT make it into the JSONB column.
func TestLayoutIngest_PrivacyReject(t *testing.T) {
	env := setupLayoutTestServer(t, "hh")
	captured := time.Now().UTC().Truncate(time.Second)

	// Hand-craft a body that includes a "pid" field on the inner window object.
	body, _ := json.Marshal(map[string]any{
		"client_device_id": env.device.ClientDeviceID,
		"snapshots": []map[string]any{
			{
				"captured_at":  captured.Format(time.RFC3339),
				"windows_hash": "hash-priv",
				"windows": []map[string]any{
					{
						"app_name":     "firefox",
						"window_title": "secret-tab",
						"x":            0, "y": 0, "w": 800, "h": 600,
						"pid":          1234, // extra field — must be dropped
						"cmdline":      "/usr/bin/firefox --private",
					},
				},
			},
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/layout-snapshots", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+env.plainKey)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "body: %s", rr.Body.String())

	var rawWindows []byte
	err := env.tdb.Pool.QueryRow(context.Background(),
		`SELECT windows::text FROM layout_snapshots WHERE device_id = $1`, env.device.ID,
	).Scan(&rawWindows)
	require.NoError(t, err)
	stored := string(rawWindows)
	assert.NotContains(t, stored, "pid", "stored JSONB must not include pid")
	assert.NotContains(t, stored, "cmdline", "stored JSONB must not include cmdline")
	assert.Contains(t, stored, "secret-tab", "the legitimate fields must be preserved")
}

func TestLayoutAt(t *testing.T) {
	env := setupLayoutTestServer(t, "ii")
	now := time.Now().UTC().Truncate(time.Second)

	// Seed via the store directly so we don't depend on the ingest handler.
	for i, off := range []time.Duration{-2 * time.Hour, -1 * time.Hour, 0} {
		_, err := env.store.InsertSnapshot(context.Background(), store.InsertSnapshotParams{
			DeviceID:    env.device.ID,
			CapturedAt:  now.Add(off),
			Windows:     []byte(`[{"app_name":"x","window_title":"y","x":0,"y":0,"w":1,"h":1}]`),
			WindowsHash: fmt.Sprintf("hash-at-%d", i),
		})
		require.NoError(t, err)
	}

	queryT := now.Add(-30 * time.Minute).Format(time.RFC3339)
	urlStr := fmt.Sprintf("/api/v1/layout-snapshots?device_id=%s&t=%s",
		env.device.ID, url.QueryEscape(queryT))
	req := httptest.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+env.jwt)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var resp struct {
		ID         string    `json:"id"`
		CapturedAt time.Time `json:"captured_at"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.WithinDuration(t, now.Add(-1*time.Hour), resp.CapturedAt, 2*time.Second)
}

func TestLayoutAt_NotFound(t *testing.T) {
	env := setupLayoutTestServer(t, "jj")
	queryT := time.Now().UTC().Format(time.RFC3339)
	urlStr := fmt.Sprintf("/api/v1/layout-snapshots?device_id=%s&t=%s",
		env.device.ID, url.QueryEscape(queryT))
	req := httptest.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+env.jwt)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestLayoutTimeline(t *testing.T) {
	env := setupLayoutTestServer(t, "kk")
	now := time.Now().UTC().Truncate(time.Second)

	for i := 0; i < 5; i++ {
		_, err := env.store.InsertSnapshot(context.Background(), store.InsertSnapshotParams{
			DeviceID:    env.device.ID,
			CapturedAt:  now.Add(time.Duration(i) * time.Minute),
			Windows:     []byte(`[{"app_name":"x","window_title":"y","x":0,"y":0,"w":1,"h":1}]`),
			WindowsHash: fmt.Sprintf("hash-tl-%d", i),
		})
		require.NoError(t, err)
	}

	from := now.Add(-1 * time.Hour).Format(time.RFC3339)
	to := now.Add(1 * time.Hour).Format(time.RFC3339)
	urlStr := fmt.Sprintf("/api/v1/layout-snapshots/timeline?device_id=%s&from=%s&to=%s",
		env.device.ID, url.QueryEscape(from), url.QueryEscape(to))
	req := httptest.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+env.jwt)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var entries []struct {
		ID           string    `json:"id"`
		CapturedAt   time.Time `json:"captured_at"`
		WindowsCount int       `json:"windows_count"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &entries))
	require.Len(t, entries, 5)
	for i := 1; i < len(entries); i++ {
		assert.True(t, !entries[i].CapturedAt.Before(entries[i-1].CapturedAt))
	}
}

func TestLayoutAt_TableMissing(t *testing.T) {
	env := setupLayoutTestServer(t, "ll")
	_, err := env.tdb.Pool.Exec(context.Background(), `DROP TABLE layout_snapshots`)
	require.NoError(t, err)

	queryT := time.Now().UTC().Format(time.RFC3339)
	urlStr := fmt.Sprintf("/api/v1/layout-snapshots?device_id=%s&t=%s",
		env.device.ID, url.QueryEscape(queryT))
	req := httptest.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+env.jwt)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "layout_snapshots table not migrated")
}

// TestLayoutAt_WrongUser exercises threat T-7-03-01: a logged-in user must
// not be able to read another user's device snapshots.
func TestLayoutAt_WrongUser(t *testing.T) {
	env := setupLayoutTestServer(t, "mm")

	// Seed a snapshot on env.device.
	_, err := env.store.InsertSnapshot(context.Background(), store.InsertSnapshotParams{
		DeviceID:    env.device.ID,
		CapturedAt:  time.Now().UTC(),
		Windows:     []byte(`[{"app_name":"x","window_title":"y","x":0,"y":0,"w":1,"h":1}]`),
		WindowsHash: "hash-wrong-user",
	})
	require.NoError(t, err)

	// Create a SECOND user with their own JWT, pointed at env.device.
	other, err := env.store.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID:    "lay-other-mm",
		Email:       "other-mm@example.com",
		DisplayName: "Other",
	})
	require.NoError(t, err)
	otherIssuer, err := auth.NewJWTIssuer("layout-handler-test-secret-32bytes!", 15*time.Minute)
	require.NoError(t, err)
	otherJWT, err := otherIssuer.Issue(other.ID, other.Email, models.Role(other.Role))
	require.NoError(t, err)

	queryT := time.Now().UTC().Format(time.RFC3339)
	urlStr := fmt.Sprintf("/api/v1/layout-snapshots?device_id=%s&t=%s",
		env.device.ID, url.QueryEscape(queryT))
	req := httptest.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+otherJWT)
	rr := httptest.NewRecorder()
	env.router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code, "wrong-user must get 403, got %d body=%s", rr.Code, rr.Body.String())
}
