package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/store"
)

var validRoles = map[string]bool{"member": true, "manager": true, "admin": true}

func userListHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := deps.Store.ListUsers(r.Context())
		if err != nil {
			deps.Logger.Error("failed to list users", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		result := make([]map[string]any, len(users))
		for i, u := range users {
			result[i] = map[string]any{
				"id":           u.ID,
				"email":        u.Email,
				"display_name": u.DisplayName,
				"role":         u.Role,
				"created_at":   u.CreatedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func userUpdateRoleHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid user ID")
			return
		}

		var req struct {
			Role string `json:"role"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if !validRoles[req.Role] {
			respondError(w, http.StatusBadRequest, "role must be one of: member, manager, admin")
			return
		}

		user, err := deps.Store.UpdateUserRole(r.Context(), userID, req.Role)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "user not found")
			} else {
				deps.Logger.Error("failed to update user role", "error", err, "user_id", userID)
				respondError(w, http.StatusInternalServerError, "failed to update user role")
			}
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"id":           user.ID,
			"email":        user.Email,
			"display_name": user.DisplayName,
			"role":         user.Role,
		})
	}
}
