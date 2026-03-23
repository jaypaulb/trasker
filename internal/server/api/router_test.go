package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaypaulb/trasker/internal/server/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_HealthEndpoint(t *testing.T) {
	deps := &api.Dependencies{} // health doesn't need deps
	router := api.NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "ok")
}

func TestRouter_NotFound(t *testing.T) {
	deps := &api.Dependencies{}
	router := api.NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/v1/nonexistent", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHealth_ReturnsStatusAndVersion(t *testing.T) {
	deps := &api.Dependencies{}
	router := api.NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var body map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Contains(t, body, "version")
	assert.Contains(t, body, "timestamp")
}
