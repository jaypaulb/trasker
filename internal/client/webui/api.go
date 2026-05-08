// internal/client/webui/api.go
package webui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// API provides REST endpoints for the local dashboard.
type API struct {
	db     *sql.DB
	logger *slog.Logger
	server *Server
}

// NewAPI creates a new API handler set.
func NewAPI(db *sql.DB, logger *slog.Logger, server *Server) *API {
	return &API{db: db, logger: logger, server: server}
}

// RegisterRoutes adds all API routes to the given mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", a.handleGetEvents)
	mux.HandleFunc("GET /api/tags", a.handleGetTags)
	mux.HandleFunc("POST /api/tags", a.handleCreateTag)
	mux.HandleFunc("POST /api/events/{id}/tag", a.handleTagEvent)
	mux.HandleFunc("POST /api/events/{id}/note", a.handleAddNote)
	mux.HandleFunc("GET /api/submissions", a.handleGetSubmissions)
	mux.HandleFunc("POST /api/submit", a.handleSubmit)
	mux.HandleFunc("GET /api/pomodoro", a.handleGetPomodoro)
	mux.HandleFunc("POST /api/pomodoro/start", a.handleStartPomodoro)
	mux.HandleFunc("POST /api/pomodoro/stop", a.handleStopPomodoro)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("PATCH /api/config", a.handleUpdateConfig)
	mux.HandleFunc("POST /api/quit", a.handleQuit)
	mux.HandleFunc("GET /api/status", a.handleStatus)
	mux.HandleFunc("GET /api/tag-rules", a.handleGetTagRules)
	mux.HandleFunc("POST /api/tag-rules", a.handleCreateTagRule)
	mux.HandleFunc("DELETE /api/tag-rules/{id}", a.handleDeleteTagRule)
	mux.HandleFunc("POST /api/tag-rules/{id}/accept", a.handleAcceptTagRule)
}

// --- Events ---

func (a *API) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	dayStart := date + "T00:00:00Z"
	dayEnd := date + "T23:59:59Z"

	rows, err := a.db.Query(
		`SELECT fe.id, fe.app_name, fe.window_title, fe.started_at, fe.ended_at,
		        fe.duration_s, fe.is_idle,
		        COALESCE(t.name, '') as tag_name,
		        COALESCE(t.color, '') as tag_color,
		        COALESCE(n.text, '') as note_text
		 FROM focus_events fe
		 LEFT JOIN event_tags et ON et.event_id = fe.id
		 LEFT JOIN tags t ON t.id = et.tag_id
		 LEFT JOIN notes n ON n.anchor_event = fe.id
		 WHERE fe.started_at >= ? AND fe.started_at <= ?
		 ORDER BY fe.started_at DESC`, dayStart, dayEnd,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to query events: "+err.Error())
		return
	}
	defer rows.Close()

	type EventResponse struct {
		ID          int64  `json:"id"`
		AppName     string `json:"app_name"`
		WindowTitle string `json:"window_title"`
		StartedAt   string `json:"started_at"`
		EndedAt     string `json:"ended_at,omitempty"`
		DurationS   *int   `json:"duration_s,omitempty"`
		IsIdle      bool   `json:"is_idle"`
		TagName     string `json:"tag_name,omitempty"`
		TagColor    string `json:"tag_color,omitempty"`
		NoteText    string `json:"note_text,omitempty"`
	}

	var events []EventResponse
	for rows.Next() {
		var e EventResponse
		var endedAt sql.NullString
		var durationS sql.NullInt64
		var isIdle int

		if err := rows.Scan(&e.ID, &e.AppName, &e.WindowTitle, &e.StartedAt,
			&endedAt, &durationS, &isIdle, &e.TagName, &e.TagColor, &e.NoteText); err != nil {
			a.jsonError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		if endedAt.Valid {
			e.EndedAt = endedAt.String
		}
		if durationS.Valid {
			d := int(durationS.Int64)
			e.DurationS = &d
		}
		e.IsIdle = isIdle == 1
		events = append(events, e)
	}

	if events == nil {
		events = []EventResponse{}
	}
	a.jsonOK(w, events)
}

// --- Tags ---

func (a *API) handleGetTags(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type TagResponse struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Color     string `json:"color"`
		CreatedAt string `json:"created_at"`
	}

	var tags []TagResponse
	for rows.Next() {
		var t TagResponse
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []TagResponse{}
	}
	a.jsonOK(w, tags)
}

func (a *API) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.Color == "" {
		a.jsonError(w, http.StatusBadRequest, "name and color are required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(`INSERT INTO tags (name, color, created_at) VALUES (?, ?, ?)`,
		req.Name, req.Color, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			a.jsonError(w, http.StatusConflict, "tag name already exists")
			return
		}
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to get tag id: "+err.Error())
		return
	}
	a.jsonOK(w, map[string]any{"id": id, "name": req.Name, "color": req.Color})
}

// --- Tag/Note Event ---

func (a *API) handleTagEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var req struct {
		TagID int64 `json:"tag_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err = a.db.Exec(
		`INSERT OR REPLACE INTO event_tags (event_id, tag_id, source) VALUES (?, ?, 'manual')`,
		eventID, req.TagID,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonOK(w, map[string]string{"status": "ok"})
}

func (a *API) handleAddNote(w http.ResponseWriter, r *http.Request) {
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Text == "" {
		a.jsonError(w, http.StatusBadRequest, "text is required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO notes (anchor_event, text, created_at) VALUES (?, ?, ?)`,
		eventID, req.Text, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to get note id: "+err.Error())
		return
	}
	a.jsonOK(w, map[string]any{"id": id})
}

// --- Submissions ---

func (a *API) handleGetSubmissions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(
		`SELECT id, server_id, submitted_at, status, retry_count
		 FROM submissions ORDER BY submitted_at DESC LIMIT 50`,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type SubResponse struct {
		ID          int64  `json:"id"`
		ServerID    string `json:"server_id,omitempty"`
		SubmittedAt string `json:"submitted_at"`
		Status      string `json:"status"`
		RetryCount  int    `json:"retry_count"`
	}

	var subs []SubResponse
	for rows.Next() {
		var s SubResponse
		var serverID sql.NullString
		if err := rows.Scan(&s.ID, &serverID, &s.SubmittedAt, &s.Status, &s.RetryCount); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if serverID.Valid {
			s.ServerID = serverID.String
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []SubResponse{}
	}
	a.jsonOK(w, subs)
}

func (a *API) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventIDs []int64 `json:"event_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.EventIDs) == 0 {
		a.jsonError(w, http.StatusBadRequest, "event_ids required")
		return
	}

	// Create submission via SubmitService (injected at runtime; here we do it inline)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := a.db.Begin()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO submissions (submitted_at, status, retry_count) VALUES (?, 'pending', 0)`, now)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	subID, err := result.LastInsertId()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to get submission id: "+err.Error())
		return
	}

	for _, eid := range req.EventIDs {
		if _, err := tx.Exec(`INSERT INTO submission_events (submission_id, event_id) VALUES (?, ?)`,
			subID, eid); err != nil {
			a.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("link event %d: %s", eid, err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonOK(w, map[string]any{"submission_id": subID, "status": "pending"})
}

// --- Pomodoro ---

func (a *API) handleGetPomodoro(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id
		 FROM pomodoro_sessions ORDER BY started_at DESC LIMIT 20`,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type PomoResponse struct {
		ID        int64  `json:"id"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at,omitempty"`
		WorkMins  int    `json:"work_mins"`
		BreakMins int    `json:"break_mins"`
		Status    string `json:"status"`
		TagID     *int64 `json:"tag_id,omitempty"`
	}

	var sessions []PomoResponse
	for rows.Next() {
		var p PomoResponse
		var endedAt sql.NullString
		var tagID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.StartedAt, &endedAt, &p.WorkMins, &p.BreakMins,
			&p.Status, &tagID); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if endedAt.Valid {
			p.EndedAt = endedAt.String
		}
		if tagID.Valid {
			v := tagID.Int64
			p.TagID = &v
		}
		sessions = append(sessions, p)
	}
	if sessions == nil {
		sessions = []PomoResponse{}
	}
	a.jsonOK(w, sessions)
}

func (a *API) handleStartPomodoro(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkMins  int    `json:"work_mins"`
		BreakMins int    `json:"break_mins"`
		TagID     *int64 `json:"tag_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.WorkMins <= 0 {
		req.WorkMins = 25
	}
	if req.BreakMins <= 0 {
		req.BreakMins = 5
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO pomodoro_sessions (started_at, work_mins, break_mins, status, tag_id, created_at)
		 VALUES (?, ?, ?, 'work', ?, ?)`,
		now, req.WorkMins, req.BreakMins, req.TagID, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to get pomodoro id: "+err.Error())
		return
	}
	a.jsonOK(w, map[string]any{"id": id, "status": "work"})
}

func (a *API) handleStopPomodoro(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`UPDATE pomodoro_sessions SET status = 'cancelled', ended_at = ?
		 WHERE status IN ('work', 'break')`, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		a.jsonError(w, http.StatusNotFound, "no active pomodoro session")
		return
	}
	a.jsonOK(w, map[string]string{"status": "cancelled"})
}

// --- Quit ---

func (a *API) handleQuit(w http.ResponseWriter, r *http.Request) {
	a.jsonOK(w, map[string]string{"status": "shutting_down"})
	// Signal shutdown after response is sent
	go a.server.RequestQuit()
}

// --- Status ---

// handleStatus returns daemon health metrics consumed by
// `trasker-client status`. Fields with no live data source are
// reported as empty strings / null timestamps; the CLI renders them
// as "unknown" / "never".
func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	type statusResponse struct {
		PID              int    `json:"pid"`
		Port             int    `json:"port"`
		StartedAt        string `json:"started_at,omitempty"`
		UptimeSeconds    int64  `json:"uptime_seconds"`
		PresenceState    string `json:"presence_state,omitempty"`
		ScreenLockState  string `json:"screen_lock_state,omitempty"`
		LastLayoutSnap   string `json:"last_layout_snapshot,omitempty"`
		LastServerSync   string `json:"last_server_sync,omitempty"`
	}

	resp := statusResponse{
		PID:  os.Getpid(),
		Port: a.server.Port(),
	}
	if started := a.server.StartedAt(); !started.IsZero() {
		resp.StartedAt = started.UTC().Format(time.RFC3339)
		resp.UptimeSeconds = int64(time.Since(started).Seconds())
	}
	if sp := a.server.Status(); sp != nil {
		resp.PresenceState = sp.PresenceState()
		resp.ScreenLockState = sp.ScreenLockState()
		if t := sp.LastLayoutSnapshot(); !t.IsZero() {
			resp.LastLayoutSnap = t.UTC().Format(time.RFC3339)
		}
		if t := sp.LastServerSync(); !t.IsZero() {
			resp.LastServerSync = t.UTC().Format(time.RFC3339)
		}
	}
	a.jsonOK(w, resp)
}

// --- Tag Rules ---

func (a *API) handleGetTagRules(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(
		`SELECT r.id, r.tag_id, t.name, r.app_pattern, r.title_pattern,
		        r.priority, r.suggested, r.hit_count
		 FROM tag_rules r
		 JOIN tags t ON t.id = r.tag_id
		 ORDER BY r.suggested ASC, r.priority DESC`)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type RuleResponse struct {
		ID           int64   `json:"id"`
		TagID        int64   `json:"tag_id"`
		TagName      string  `json:"tag_name"`
		AppPattern   string  `json:"app_pattern"`
		TitlePattern *string `json:"title_pattern,omitempty"`
		Priority     int     `json:"priority"`
		Suggested    bool    `json:"suggested"`
		HitCount     int     `json:"hit_count"`
	}

	var rules []RuleResponse
	for rows.Next() {
		var rule RuleResponse
		var titlePattern sql.NullString
		var suggested int
		if err := rows.Scan(&rule.ID, &rule.TagID, &rule.TagName, &rule.AppPattern,
			&titlePattern, &rule.Priority, &suggested, &rule.HitCount); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if titlePattern.Valid {
			rule.TitlePattern = &titlePattern.String
		}
		rule.Suggested = suggested == 1
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []RuleResponse{}
	}
	a.jsonOK(w, rules)
}

func (a *API) handleCreateTagRule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TagID        int64   `json:"tag_id"`
		AppPattern   string  `json:"app_pattern"`
		TitlePattern *string `json:"title_pattern,omitempty"`
		Priority     int     `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TagID == 0 || req.AppPattern == "" {
		a.jsonError(w, http.StatusBadRequest, "tag_id and app_pattern are required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO tag_rules (tag_id, app_pattern, title_pattern, priority, suggested, hit_count, created_at)
		 VALUES (?, ?, ?, ?, 0, 0, ?)`,
		req.TagID, req.AppPattern, req.TitlePattern, req.Priority, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, _ := result.LastInsertId()
	a.jsonOK(w, map[string]any{"id": id, "status": "created"})
}

func (a *API) handleDeleteTagRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	result, err := a.db.Exec(`DELETE FROM tag_rules WHERE id = ?`, id)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		a.jsonError(w, http.StatusNotFound, "rule not found")
		return
	}
	a.jsonOK(w, map[string]string{"status": "deleted"})
}

func (a *API) handleAcceptTagRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid rule id")
		return
	}
	result, err := a.db.Exec(`UPDATE tag_rules SET suggested = 0 WHERE id = ? AND suggested = 1`, id)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		a.jsonError(w, http.StatusNotFound, "rule not found or not suggested")
		return
	}
	a.jsonOK(w, map[string]string{"status": "accepted"})
}

// --- Config ---

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	row := a.db.QueryRow(
		`SELECT device_id, server_url, tracking_on, autostart, presence_intervals, pomodoro_defaults
		 FROM config WHERE id = 1`,
	)

	type ConfigResponse struct {
		DeviceID          string `json:"device_id"`
		ServerURL         string `json:"server_url"`
		TrackingOn        bool   `json:"tracking_on"`
		Autostart         bool   `json:"autostart"`
		PresenceIntervals string `json:"presence_intervals"`
		PomodoroDefaults  string `json:"pomodoro_defaults"`
	}

	var c ConfigResponse
	var trackingOn, autostart int
	if err := row.Scan(&c.DeviceID, &c.ServerURL, &trackingOn, &autostart,
		&c.PresenceIntervals, &c.PomodoroDefaults); err != nil {
		a.jsonError(w, http.StatusInternalServerError, "config not found: "+err.Error())
		return
	}
	c.TrackingOn = trackingOn == 1
	c.Autostart = autostart == 1

	a.jsonOK(w, c)
}

func (a *API) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Supported fields for update
	allowed := map[string]string{
		"tracking_on":        "tracking_on",
		"autostart":          "autostart",
		"presence_intervals": "presence_intervals",
		"pomodoro_defaults":  "pomodoro_defaults",
	}

	for key, val := range req {
		col, ok := allowed[key]
		if !ok {
			continue
		}
		if _, err := a.db.Exec(
			fmt.Sprintf(`UPDATE config SET %s = ? WHERE id = 1`, col), val,
		); err != nil {
			a.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("update %s: %s", key, err))
			return
		}
	}

	a.jsonOK(w, map[string]string{"status": "ok"})
}

// --- Helpers ---

func (a *API) jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func (a *API) jsonError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
