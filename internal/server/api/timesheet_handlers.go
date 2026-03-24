package api

import (
	"net/http"
	"time"

	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

func timesheetSubmitHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		apiKeyID, ok := auth.APIKeyIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "no API key context")
			return
		}

		var req struct {
			ClientDeviceID string `json:"client_device_id"`
			Entries        []struct {
				Tag        string `json:"tag"`
				StartedAt  string `json:"started_at"`
				EndedAt    string `json:"ended_at"`
				DurationS  int    `json:"duration_s"`
				Notes      string `json:"notes,omitempty"`
				AppSummary string `json:"app_summary,omitempty"`
			} `json:"entries"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.ClientDeviceID == "" {
			respondError(w, http.StatusBadRequest, "client_device_id is required")
			return
		}
		if len(req.Entries) == 0 {
			respondError(w, http.StatusBadRequest, "at least one entry is required")
			return
		}

		// Resolve device
		device, err := deps.Store.GetDeviceByClientID(r.Context(), req.ClientDeviceID, apiKeyID)
		if err != nil {
			deps.Logger.Error("failed to resolve device", "error", err, "client_device_id", req.ClientDeviceID)
			respondError(w, http.StatusBadRequest, "device not registered")
			return
		}

		// Parse entries
		entries := make([]store.CreateTimesheetEntryParams, 0, len(req.Entries))
		for _, e := range req.Entries {
			startedAt, err := time.Parse(time.RFC3339, e.StartedAt)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid started_at format, use RFC3339")
				return
			}
			endedAt, err := time.Parse(time.RFC3339, e.EndedAt)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid ended_at format, use RFC3339")
				return
			}

			entry := store.CreateTimesheetEntryParams{
				Tag:       e.Tag,
				StartedAt: startedAt,
				EndedAt:   endedAt,
				DurationS: e.DurationS,
			}
			if e.Notes != "" {
				entry.Notes = &e.Notes
			}
			if e.AppSummary != "" {
				entry.AppSummary = &e.AppSummary
			}
			entries = append(entries, entry)
		}

		ts, err := deps.Store.CreateTimesheet(r.Context(), store.CreateTimesheetParams{
			UserID:      userID,
			DeviceID:    device.ID,
			SubmittedAt: time.Now().UTC(),
			Entries:     entries,
		})
		if err != nil {
			deps.Logger.Error("failed to create timesheet", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "failed to create timesheet")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]any{
			"id":           ts.ID,
			"submitted_at": ts.SubmittedAt,
			"entry_count":  len(ts.Entries),
		})
	}
}

func timesheetListOwnHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		timesheets, err := deps.Store.ListTimesheetsByUser(r.Context(), userID)
		if err != nil {
			deps.Logger.Error("failed to list timesheets", "error", err, "user_id", userID)
			respondError(w, http.StatusInternalServerError, "failed to list timesheets")
			return
		}

		result := make([]map[string]any, len(timesheets))
		for i, ts := range timesheets {
			result[i] = map[string]any{
				"id":           ts.ID,
				"submitted_at": ts.SubmittedAt,
				"created_at":   ts.CreatedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func timesheetListTeamHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		timesheets, err := deps.Store.ListTimesheetsAll(r.Context(), store.TimesheetFilters{})
		if err != nil {
			deps.Logger.Error("failed to list team timesheets", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to list team timesheets")
			return
		}

		result := make([]map[string]any, len(timesheets))
		for i, ts := range timesheets {
			result[i] = map[string]any{
				"id":           ts.ID,
				"user_id":      ts.UserID,
				"submitted_at": ts.SubmittedAt,
				"created_at":   ts.CreatedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}
