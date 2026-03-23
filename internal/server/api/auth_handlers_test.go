package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRefresh_ValidToken(t *testing.T) {
	secret := "test-secret-32-bytes-minimum!!!!!"
	issuer, err := auth.NewJWTIssuer(secret, 15*time.Minute)
	require.NoError(t, err)

	deps := &api.Dependencies{JWTIssuer: issuer}
	router := api.NewRouter(deps)

	userID := uuid.New()
	token, err := issuer.Issue(userID, "alice@example.com", "admin")
	require.NoError(t, err)

	body, _ := json.Marshal(map[string]string{"token": token})
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp["token"])
}
