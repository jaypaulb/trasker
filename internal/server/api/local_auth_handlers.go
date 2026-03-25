package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
	"github.com/jaypaulb/trasker/internal/shared/models"
	"golang.org/x/crypto/bcrypt"
)

// DefaultAdminEmail is the bootstrap admin account email.
const DefaultAdminEmail = "admin@localhost"

// DefaultAdminPassword is the well-known default password that must be changed on first login.
const DefaultAdminPassword = "trasker-admin"

// authConfigHandler returns the server's auth capabilities so the SPA knows what to show.
// Checks both the runtime OIDC config and the org_settings table.
func authConfigHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		oidcEnabled := deps.OIDCConfig != nil && deps.OIDCConfig.Provider != nil

		// Also check org_settings — admin may have configured OIDC via the UI
		if !oidcEnabled && deps.Store != nil {
			settings, err := deps.Store.GetOrgSettings(r.Context())
			if err == nil && settings.EntraTenant != "" && settings.EntraClient != "" {
				oidcEnabled = true
			}
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"oidc_enabled":  oidcEnabled,
			"local_enabled": true,
		})
	}
}

// localLoginHandler authenticates a user with email + password.
func localLoginHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Email == "" || req.Password == "" {
			respondError(w, http.StatusBadRequest, "email and password are required")
			return
		}

		user, err := deps.Store.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusUnauthorized, "invalid email or password")
			} else {
				deps.Logger.Error("failed to look up user by email", "error", err)
				respondError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}

		if user.PasswordHash == nil {
			respondError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
			respondError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}

		token, err := deps.JWTIssuer.Issue(user.ID, user.Email, models.Role(user.Role))
		if err != nil {
			deps.Logger.Error("failed to issue token", "error", err, "user_id", user.ID)
			respondError(w, http.StatusInternalServerError, "failed to issue token")
			return
		}

		// Build response matching AuthTokens shape expected by SPA
		expiresAt := time.Now().Add(15 * time.Minute).Unix()

		respondJSON(w, http.StatusOK, map[string]any{
			"tokens": map[string]any{
				"access_token":  token,
				"refresh_token": token, // same JWT; refresh endpoint will reissue
				"expires_at":    expiresAt,
			},
			"user": map[string]any{
				"id":                    user.ID,
				"email":                 user.Email,
				"display_name":          user.DisplayName,
				"role":                  user.Role,
				"force_password_change": user.ForcePasswordChange,
			},
		})
	}
}

// changePasswordHandler lets an authenticated user change their password.
func changePasswordHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.NewPassword == "" || len(req.NewPassword) < 8 {
			respondError(w, http.StatusBadRequest, "new password must be at least 8 characters")
			return
		}

		user, err := deps.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			deps.Logger.Error("failed to get user for password change", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if user.PasswordHash == nil {
			respondError(w, http.StatusBadRequest, "account does not use password authentication")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			respondError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			deps.Logger.Error("failed to hash new password", "error", err)
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if _, err := deps.Store.UpdateUserPassword(r.Context(), userID, string(hash)); err != nil {
			deps.Logger.Error("failed to update password", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "failed to update password")
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
	}
}
