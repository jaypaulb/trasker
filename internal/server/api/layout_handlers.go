package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaypaulb/trasker/internal/server/auth"
	"github.com/jaypaulb/trasker/internal/server/store"
)

// layoutMaxBodyBytes caps the request body for layout ingest at 4MB.
// Estimated worst case: 256 windows * ~200 bytes/window * 4× margin.
// The shared decodeJSON helper uses 1MB which is too tight for layout payloads.
const layoutMaxBodyBytes = 4 << 20

// layoutSchemaMissingMsg is the operator-facing 503 body emitted when the
// layout_snapshots table has not been migrated. Mentions the file to apply
// so the operator can recover without source-diving.
const layoutSchemaMissingMsg = "layout_snapshots table not migrated; apply migrations/004_layout_snapshots.up.sql"

// layoutWindowReq matches the inner Window object the client sends.
// Exactly six fields — any other field present in the inbound JSON is
// silently dropped by json.Decode AND we re-marshal before storage to enforce
// the contract end-to-end. Privacy is a one-way door (D-16).
type layoutWindowReq struct {
	AppName     string `json:"app_name"`
	WindowTitle string `json:"window_title"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
}

type layoutSnapshotReq struct {
	CapturedAt  string            `json:"captured_at"`
	WindowsHash string            `json:"windows_hash"`
	Windows     []layoutWindowReq `json:"windows"`
}

type layoutIngestReq struct {
	ClientDeviceID string              `json:"client_device_id"`
	Snapshots      []layoutSnapshotReq `json:"snapshots"`
}

// decodeLayoutJSON applies the 4MB cap and decodes into v. Returns the raw
// MaxBytesReader error if the cap is exceeded so the caller can map to 413.
func decodeLayoutJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, layoutMaxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// layoutIngestHandler handles POST /api/v1/layout-snapshots under API-key auth.
//
// Idempotency: the store's ON CONFLICT DO NOTHING returns (nil, nil) on dedup;
// we count those as "deduped" and still respond 201. Client retries on lost
// responses are safe.
func layoutIngestHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		apiKeyID, ok := auth.APIKeyIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "no API key context")
			return
		}

		var req layoutIngestReq
		if err := decodeLayoutJSON(r, &req); err != nil {
			// MaxBytesReader produces a *http.MaxBytesError on overflow.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				respondError(w, http.StatusRequestEntityTooLarge, "request body exceeds 4MB layout cap")
				return
			}
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.ClientDeviceID == "" {
			respondError(w, http.StatusBadRequest, "client_device_id is required")
			return
		}
		if len(req.Snapshots) == 0 {
			respondError(w, http.StatusBadRequest, "at least one snapshot is required")
			return
		}

		// Schema-missing degrade. Cheap probe — runs once per request.
		has, err := deps.Store.HasLayoutSnapshotsTable(r.Context())
		if err != nil {
			deps.Logger.Error("check layout_snapshots table", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to check schema")
			return
		}
		if !has {
			respondError(w, http.StatusServiceUnavailable, layoutSchemaMissingMsg)
			return
		}

		device, err := deps.Store.GetDeviceByClientID(r.Context(), req.ClientDeviceID, apiKeyID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusBadRequest, "device not registered")
				return
			}
			deps.Logger.Error("failed to resolve device", "error", err, "client_device_id", req.ClientDeviceID)
			respondError(w, http.StatusInternalServerError, "failed to look up device")
			return
		}

		var received, deduped int
		for _, snap := range req.Snapshots {
			capturedAt, err := time.Parse(time.RFC3339, snap.CapturedAt)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid captured_at format, use RFC3339")
				return
			}
			if snap.WindowsHash == "" {
				respondError(w, http.StatusBadRequest, "windows_hash is required")
				return
			}

			// Re-marshal the cleaned windows so storage gets exactly the 6
			// allowed fields. Even if json.Decode tolerated unknown fields
			// (it does, by default), this guarantees what reaches JSONB.
			cleanWindows, err := json.Marshal(snap.Windows)
			if err != nil {
				deps.Logger.Error("re-marshal windows", "error", err)
				respondError(w, http.StatusInternalServerError, "failed to serialize windows")
				return
			}

			row, err := deps.Store.InsertSnapshot(r.Context(), store.InsertSnapshotParams{
				DeviceID:    device.ID,
				CapturedAt:  capturedAt,
				Windows:     cleanWindows,
				WindowsHash: snap.WindowsHash,
			})
			if err != nil {
				deps.Logger.Error("insert layout snapshot", "error", err, "device_id", device.ID)
				respondError(w, http.StatusInternalServerError, "failed to insert snapshot")
				return
			}
			received++
			if row == nil {
				deduped++
			}
		}

		respondJSON(w, http.StatusCreated, map[string]any{
			"received": received,
			"deduped":  deduped,
		})
	}
}

// authorizeDeviceForUser fetches the device and verifies it belongs to the
// authenticated user. Returns (device, true) on success; on failure writes
// the appropriate response and returns (nil, false).
func authorizeDeviceForUser(deps *Dependencies, w http.ResponseWriter, r *http.Request, deviceID, userID uuid.UUID) (*store.Device, bool) {
	device, err := deps.Store.GetDeviceByID(r.Context(), deviceID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "device not found")
			return nil, false
		}
		deps.Logger.Error("authorize device", "error", err, "device_id", deviceID)
		respondError(w, http.StatusInternalServerError, "failed to look up device")
		return nil, false
	}
	if device.UserID != userID {
		// Threat T-7-03-01 — cross-user device read.
		respondError(w, http.StatusForbidden, "device does not belong to authenticated user")
		return nil, false
	}
	return device, true
}

// layoutAtHandler handles GET /api/v1/layout-snapshots?device_id=&t= under JWT auth.
func layoutAtHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		deviceID, err := uuid.Parse(r.URL.Query().Get("device_id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid or missing device_id")
			return
		}

		// Schema-missing degrade BEFORE device lookup so operators get the
		// actionable message rather than a 500 from missing FK target.
		has, err := deps.Store.HasLayoutSnapshotsTable(r.Context())
		if err != nil {
			deps.Logger.Error("check layout_snapshots table", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to check schema")
			return
		}
		if !has {
			respondError(w, http.StatusServiceUnavailable, layoutSchemaMissingMsg)
			return
		}

		device, ok := authorizeDeviceForUser(deps, w, r, deviceID, userID)
		if !ok {
			return
		}

		t, err := time.Parse(time.RFC3339, r.URL.Query().Get("t"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid or missing t (RFC3339)")
			return
		}

		snap, err := deps.Store.GetSnapshotAt(r.Context(), device.ID, t)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "no snapshot at-or-before t")
				return
			}
			deps.Logger.Error("get snapshot at", "error", err, "device_id", device.ID)
			respondError(w, http.StatusInternalServerError, "failed to fetch snapshot")
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"id":           snap.ID,
			"device_id":    snap.DeviceID,
			"captured_at":  snap.CapturedAt,
			"windows":      snap.Windows, // RawMessage embeds verbatim
			"windows_hash": snap.WindowsHash,
			"tier":         snap.Tier,
			"created_at":   snap.CreatedAt,
		})
	}
}

// layoutTimelineHandler handles GET /api/v1/layout-snapshots/timeline under JWT auth.
func layoutTimelineHandler(deps *Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFrom(r.Context())
		if !ok {
			respondError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		deviceID, err := uuid.Parse(r.URL.Query().Get("device_id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid or missing device_id")
			return
		}

		has, err := deps.Store.HasLayoutSnapshotsTable(r.Context())
		if err != nil {
			deps.Logger.Error("check layout_snapshots table", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to check schema")
			return
		}
		if !has {
			respondError(w, http.StatusServiceUnavailable, layoutSchemaMissingMsg)
			return
		}

		device, ok := authorizeDeviceForUser(deps, w, r, deviceID, userID)
		if !ok {
			return
		}

		now := time.Now().UTC()
		from := todayUTCMidnight(now)
		to := now

		if v := r.URL.Query().Get("from"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid from (RFC3339)")
				return
			}
			from = t
		}
		if v := r.URL.Query().Get("to"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid to (RFC3339)")
				return
			}
			to = t
		}

		limit, offset := parsePagination(r)
		entries, err := deps.Store.ListTimestamps(r.Context(), device.ID, from, to, limit, offset)
		if err != nil {
			deps.Logger.Error("list timestamps", "error", err, "device_id", device.ID)
			respondError(w, http.StatusInternalServerError, "failed to list timeline")
			return
		}

		// Render with explicit field names so the dashboard contract is stable.
		out := make([]map[string]any, len(entries))
		for i, e := range entries {
			out[i] = map[string]any{
				"id":            e.ID,
				"captured_at":   e.CapturedAt,
				"windows_count": e.WindowsCount,
			}
		}
		respondJSON(w, http.StatusOK, out)
	}
}

// todayUTCMidnight returns 00:00 UTC of the same calendar day as t.
// Used as the default `from` in the timeline endpoint per UI-SPEC mode A.
func todayUTCMidnight(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
