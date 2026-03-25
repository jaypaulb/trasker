// internal/server/api/admin_handlers.go
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

func adminDeleteEntryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid entry ID")
			return
		}

		adminID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			deps.Logger.Error("admin user ID missing from context")
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Get entry before deleting (for audit log)
		entry, err := deps.Store.GetTimesheetEntryByID(r.Context(), entryID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "entry not found")
			} else {
				deps.Logger.Error("failed to get entry for deletion", "error", err, "entry_id", entryID)
				respondError(w, http.StatusInternalServerError, "failed to look up entry")
			}
			return
		}

		oldValue, _ := json.Marshal(entry)

		if err := deps.Store.DeleteTimesheetEntry(r.Context(), entryID); err != nil {
			deps.Logger.Error("failed to delete entry", "error", err, "entry_id", entryID)
			respondError(w, http.StatusInternalServerError, "failed to delete entry")
			return
		}

		// Audit log
		if _, err := deps.Store.CreateAuditLog(r.Context(), store.CreateAuditLogParams{
			AdminID:    adminID,
			Action:     "entry.delete",
			TargetType: "timesheet_entry",
			TargetID:   entryID,
			OldValue:   oldValue,
		}); err != nil {
			deps.Logger.Error("failed to write audit log", "action", "entry.delete", "error", err)
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

func adminEditEntryHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entryID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid entry ID")
			return
		}

		adminID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			deps.Logger.Error("admin user ID missing from context")
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Get old entry for audit
		oldEntry, err := deps.Store.GetTimesheetEntryByID(r.Context(), entryID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "entry not found")
			} else {
				deps.Logger.Error("failed to get entry for edit", "error", err, "entry_id", entryID)
				respondError(w, http.StatusInternalServerError, "failed to look up entry")
			}
			return
		}
		oldValue, _ := json.Marshal(oldEntry)

		var req struct {
			Tag       *string `json:"tag,omitempty"`
			DurationS *int    `json:"duration_s,omitempty"`
			Notes     *string `json:"notes,omitempty"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		updated, err := deps.Store.UpdateTimesheetEntry(r.Context(), entryID, store.UpdateTimesheetEntryParams{
			Tag:       req.Tag,
			DurationS: req.DurationS,
			Notes:     req.Notes,
		})
		if err != nil {
			deps.Logger.Error("failed to update entry", "error", err, "entry_id", entryID)
			respondError(w, http.StatusInternalServerError, "failed to update entry")
			return
		}

		newValue, _ := json.Marshal(updated)

		// Audit log
		if _, err := deps.Store.CreateAuditLog(r.Context(), store.CreateAuditLogParams{
			AdminID:    adminID,
			Action:     "entry.update",
			TargetType: "timesheet_entry",
			TargetID:   entryID,
			OldValue:   oldValue,
			NewValue:   newValue,
		}); err != nil {
			deps.Logger.Error("failed to write audit log", "action", "entry.update", "error", err)
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"id":         updated.ID,
			"tag":        updated.Tag,
			"started_at": updated.StartedAt,
			"ended_at":   updated.EndedAt,
			"duration_s": updated.DurationS,
			"notes":      updated.Notes,
		})
	}
}

func adminRevokeKeyHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid key ID")
			return
		}

		adminID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			deps.Logger.Error("admin user ID missing from context")
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if err := deps.Store.RevokeAPIKey(r.Context(), keyID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "API key not found")
			} else {
				deps.Logger.Error("failed to revoke API key", "error", err, "key_id", keyID)
				respondError(w, http.StatusInternalServerError, "failed to revoke API key")
			}
			return
		}

		// Audit log
		if _, err := deps.Store.CreateAuditLog(r.Context(), store.CreateAuditLogParams{
			AdminID:    adminID,
			Action:     "key.revoke",
			TargetType: "api_key",
			TargetID:   keyID,
		}); err != nil {
			deps.Logger.Error("failed to write audit log", "action", "key.revoke", "error", err)
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
	}
}

func adminAuditLogHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		filters := store.AuditLogFilters{
			Limit:  limit,
			Offset: offset,
		}

		if action := r.URL.Query().Get("action"); action != "" {
			filters.Action = &action
		}
		if targetType := r.URL.Query().Get("target_type"); targetType != "" {
			filters.TargetType = &targetType
		}

		logs, err := deps.Store.ListAuditLogs(r.Context(), filters)
		if err != nil {
			deps.Logger.Error("failed to list audit logs", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to list audit logs")
			return
		}

		result := make([]map[string]any, len(logs))
		for i, l := range logs {
			result[i] = map[string]any{
				"id":          l.ID,
				"admin_id":    l.AdminID,
				"action":      l.Action,
				"target_type": l.TargetType,
				"target_id":   l.TargetID,
				"old_value":   l.OldValue,
				"new_value":   l.NewValue,
				"created_at":  l.CreatedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func adminListKeysHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userIDStr := r.URL.Query().Get("user_id")
		if userIDStr == "" {
			respondError(w, http.StatusBadRequest, "user_id query parameter required")
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid user_id")
			return
		}

		keys, err := deps.Store.ListAPIKeysByUser(r.Context(), userID)
		if err != nil {
			deps.Logger.Error("failed to list API keys", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "failed to list API keys")
			return
		}

		result := make([]map[string]any, len(keys))
		for i, k := range keys {
			result[i] = map[string]any{
				"id":           k.ID,
				"user_id":      k.UserID,
				"key_prefix":   k.KeyPrefix,
				"last_used_at": k.LastUsedAt,
				"expires_at":   k.ExpiresAt,
				"revoked":      k.Revoked,
				"created_at":   k.CreatedAt,
				"revoked_at":   k.RevokedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func adminGetSettingsHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := deps.Store.GetOrgSettings(r.Context())
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "org settings not configured")
			} else {
				deps.Logger.Error("failed to get org settings", "error", err)
				respondError(w, http.StatusInternalServerError, "failed to get settings")
			}
			return
		}

		// Return entra_secret as masked (never expose full secret to browser)
		entraSecretMasked := ""
		if settings.EntraSecret != "" {
			entraSecretMasked = "********"
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"org_name":        settings.OrgName,
			"entra_tenant":    settings.EntraTenant,
			"entra_client":    settings.EntraClient,
			"entra_secret":    entraSecretMasked,
			"key_expiry_days": settings.KeyExpiryDays,
			"updated_at":      settings.UpdatedAt,
		})
	}
}

func adminUpdateSettingsHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			OrgName       *string `json:"org_name,omitempty"`
			EntraTenant   *string `json:"entra_tenant,omitempty"`
			EntraClient   *string `json:"entra_client,omitempty"`
			EntraSecret   *string `json:"entra_secret,omitempty"`
			KeyExpiryDays *int    `json:"key_expiry_days,omitempty"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Don't overwrite secret with the masked placeholder
		if req.EntraSecret != nil && *req.EntraSecret == "********" {
			req.EntraSecret = nil
		}

		settings, err := deps.Store.UpsertOrgSettings(r.Context(), store.UpdateOrgSettingsParams{
			OrgName:       req.OrgName,
			EntraTenant:   req.EntraTenant,
			EntraClient:   req.EntraClient,
			EntraSecret:   req.EntraSecret,
			KeyExpiryDays: req.KeyExpiryDays,
		})
		if err != nil {
			deps.Logger.Error("failed to update settings", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}

		// If OIDC settings changed, clear cached config so it re-initializes on next login
		if req.EntraTenant != nil || req.EntraClient != nil || req.EntraSecret != nil {
			deps.OIDCConfig = nil
			deps.Logger.Info("OIDC config cleared — will re-initialize on next login attempt")
		}

		entraSecretMasked := ""
		if settings.EntraSecret != "" {
			entraSecretMasked = "********"
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"org_name":        settings.OrgName,
			"entra_tenant":    settings.EntraTenant,
			"entra_client":    settings.EntraClient,
			"entra_secret":    entraSecretMasked,
			"key_expiry_days": settings.KeyExpiryDays,
			"updated_at":      settings.UpdatedAt,
		})
	}
}
