package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/stretchr/testify/assert"
)

func TestRequireRole_AdminAllowed(t *testing.T) {
	handler := auth.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := auth.WithUserID(req.Context(), uuid.New())
	ctx = auth.WithUserRole(ctx, "admin")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRequireRole_MemberDenied(t *testing.T) {
	handler := auth.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := auth.WithUserID(req.Context(), uuid.New())
	ctx = auth.WithUserRole(ctx, "member")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestRequireRole_ManagerOrAdmin(t *testing.T) {
	handler := auth.RequireRole("manager", "admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Manager allowed
	req := httptest.NewRequest("GET", "/", nil)
	ctx := auth.WithUserID(req.Context(), uuid.New())
	ctx = auth.WithUserRole(ctx, "manager")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Member denied
	req2 := httptest.NewRequest("GET", "/", nil)
	ctx2 := auth.WithUserID(req2.Context(), uuid.New())
	ctx2 = auth.WithUserRole(ctx2, "member")
	req2 = req2.WithContext(ctx2)

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusForbidden, rr2.Code)
}

func TestRequireRole_NoRoleInContext(t *testing.T) {
	handler := auth.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
