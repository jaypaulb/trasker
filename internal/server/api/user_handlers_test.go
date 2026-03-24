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
)

func TestUserHandler_ListUsers(t *testing.T) {
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
		EntraOID: "user-admin-oid", Email: "admin@example.com", DisplayName: "Admin",
	})
	require.NoError(t, err)
	_, err = s.UpdateUserRole(context.Background(), admin.ID, "admin")
	require.NoError(t, err)

	// Create some other users
	_, err = s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "user-member-oid", Email: "member@example.com", DisplayName: "Member",
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, JWTIssuer: issuer}
	router := api.NewRouter(deps)

	token, err := issuer.Issue(admin.ID, admin.Email, "admin")
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp []map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp, 2)
}

func TestUserHandler_UpdateRole(t *testing.T) {
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
		EntraOID: "role-admin-oid", Email: "roleadmin@example.com", DisplayName: "Role Admin",
	})
	require.NoError(t, err)
	_, err = s.UpdateUserRole(context.Background(), admin.ID, "admin")
	require.NoError(t, err)

	member, err := s.CreateUser(context.Background(), store.CreateUserParams{
		EntraOID: "role-member-oid", Email: "rolemember@example.com", DisplayName: "Role Member",
	})
	require.NoError(t, err)

	deps := &api.Dependencies{Store: s, JWTIssuer: issuer}
	router := api.NewRouter(deps)

	token, err := issuer.Issue(admin.ID, admin.Email, "admin")
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]string{"role": "manager"})
	req := httptest.NewRequest("PATCH", "/api/v1/users/"+member.ID.String(), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]any
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "manager", resp["role"])
}
