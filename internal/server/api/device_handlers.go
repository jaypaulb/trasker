package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

func deviceRegisterHandler(deps *Dependencies) http.HandlerFunc {
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
			OS             string `json:"os"`
			Hostname       string `json:"hostname,omitempty"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.ClientDeviceID == "" || req.OS == "" {
			respondError(w, http.StatusBadRequest, "client_device_id and os are required")
			return
		}

		device, err := deps.Store.UpsertDevice(r.Context(), store.UpsertDeviceParams{
			UserID:         userID,
			APIKeyID:       apiKeyID,
			ClientDeviceID: req.ClientDeviceID,
			OS:             req.OS,
		})
		if err != nil {
			deps.Logger.Error("failed to register device", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to register device")
			return
		}

		respondJSON(w, http.StatusCreated, map[string]any{
			"id":               device.ID,
			"client_device_id": device.ClientDeviceID,
			"os":               device.OS,
			"device_name":      device.DeviceName,
			"last_seen_at":     device.LastSeenAt,
			"created_at":       device.CreatedAt,
		})
	}
}

func deviceListHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		devices, err := deps.Store.ListDevicesByUser(r.Context(), userID)
		if err != nil {
			deps.Logger.Error("failed to list devices", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to list devices")
			return
		}

		result := make([]map[string]any, len(devices))
		for i, d := range devices {
			result[i] = map[string]any{
				"id":               d.ID,
				"client_device_id": d.ClientDeviceID,
				"os":               d.OS,
				"device_name":      d.DeviceName,
				"last_seen_at":     d.LastSeenAt,
				"created_at":       d.CreatedAt,
			}
		}

		respondJSON(w, http.StatusOK, result)
	}
}

func deviceUpdateHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid device ID")
			return
		}

		var req struct {
			DeviceName string `json:"device_name"`
		}
		if err := decodeJSON(r, &req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.DeviceName == "" {
			respondError(w, http.StatusBadRequest, "device_name is required")
			return
		}

		device, err := deps.Store.UpdateDeviceName(r.Context(), deviceID, req.DeviceName)
		if err != nil {
			deps.Logger.Error("failed to update device name", "error", err, "device_id", deviceID)
			respondError(w, http.StatusNotFound, "device not found")
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"id":               device.ID,
			"client_device_id": device.ClientDeviceID,
			"os":               device.OS,
			"device_name":      device.DeviceName,
			"last_seen_at":     device.LastSeenAt,
		})
	}
}
