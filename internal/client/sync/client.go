// internal/client/sync/client.go
package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jaypaulb/trasker/internal/shared/models"
)

// DeviceRegistration is the wire-format payload for POST /api/v1/devices.
// This mirrors models.DeviceRegistrationRequest but lives here as the
// sync package's own wire type to avoid coupling the HTTP layer to the
// shared models package.
type DeviceRegistration struct {
	ClientDeviceID string `json:"client_device_id"`
	OS             string `json:"os"`
	Hostname       string `json:"hostname"`
}

// TimesheetEntry is the wire-format representation of a single entry in a
// timesheet submission. Timestamps are RFC 3339 strings for JSON serialization.
// The corresponding domain type is models.TimesheetEntry which uses time.Time.
type TimesheetEntry struct {
	Tag        string `json:"tag"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	DurationS  int    `json:"duration_s"`
	Notes      string `json:"notes,omitempty"`
	AppSummary string `json:"app_summary,omitempty"`
}

// TimesheetEntryFromModel converts a domain models.TimesheetEntry to the
// wire-format TimesheetEntry used for server communication.
func TimesheetEntryFromModel(m models.TimesheetEntry) TimesheetEntry {
	return TimesheetEntry{
		Tag:        m.Tag,
		StartedAt:  m.StartedAt.Format(time.RFC3339),
		EndedAt:    m.EndedAt.Format(time.RFC3339),
		DurationS:  m.DurationS,
		Notes:      m.Notes,
		AppSummary: m.AppSummary,
	}
}

// TimesheetSubmission is the payload for POST /api/v1/timesheets.
type TimesheetSubmission struct {
	ClientDeviceID string           `json:"client_device_id"`
	Entries        []TimesheetEntry `json:"entries"`
}

// TimesheetResponse is returned by the server after submission.
type TimesheetResponse struct {
	ID          string `json:"id"`
	SubmittedAt string `json:"submitted_at"`
}

// ErrorResponse represents a server error.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("server error %d: %s", e.Code, e.Message)
}

// serverErrorBody mirrors the wire-format `{"error": "...", "message": "..."}`
// shape returned by the trasker server's auth middleware and respondError
// helper. The optional `message` field carries operator-friendly guidance
// (e.g. "Please download a new client from your Trasker dashboard").
type serverErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ErrKeyExpired is returned ONLY when the server explicitly indicates the API
// key has expired (response body `{"error": "API key has expired", ...}`).
// Previously this sentinel was returned for ANY 401, which caused the client
// to lie to the user when the real reason was a missing/invalid header,
// invalid key format, or bcrypt mismatch (Bug-3).
var ErrKeyExpired = fmt.Errorf("API key expired — download a new client from your Trasker dashboard")

// ErrKeyRevoked is returned when the server indicates the API key has been
// revoked. The auth middleware emits this as a 401 with body
// `{"error": "API key has been revoked"}`; older code paths may surface it
// as a 403. Both map here.
var ErrKeyRevoked = fmt.Errorf("API key revoked — contact your administrator")

// ErrKeyInvalid is returned for 401 responses where the cause is something
// other than "expired" or "revoked" — missing Authorization header, invalid
// header format, malformed key, or bcrypt mismatch. These are typically
// recoverable only by re-downloading or re-installing the client, but the
// remediation is different from "expired" so they get their own sentinel.
var ErrKeyInvalid = fmt.Errorf("API key invalid — check your client configuration or re-download the client")

// ErrPermissionDenied is returned for 403 responses where the cause is not
// an explicit revoke — the authenticated key lacks permission for the
// requested resource (e.g. role-based access control rejection).
var ErrPermissionDenied = fmt.Errorf("permission denied — your account does not have access to this resource")

// Client handles HTTP communication with the Trasker server.
type Client struct {
	httpClient *http.Client
	serverURL  string
	apiKey     string
}

// NewClient creates a sync client with the given server URL and API key.
func NewClient(serverURL, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		serverURL:  serverURL,
		apiKey:     apiKey,
	}
}

// RegisterDevice registers this device with the server. Idempotent (server upserts).
func (c *Client) RegisterDevice(ctx context.Context, reg DeviceRegistration) error {
	_, err := c.post(ctx, "/api/v1/devices", reg)
	return err
}

// SubmitTimesheet sends a timesheet to the server. Returns the server-assigned ID.
func (c *Client) SubmitTimesheet(ctx context.Context, sub TimesheetSubmission) (*TimesheetResponse, error) {
	body, err := c.post(ctx, "/api/v1/timesheets", sub)
	if err != nil {
		return nil, err
	}

	var resp TimesheetResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sync client: unmarshal response: %w", err)
	}
	return &resp, nil
}

// HealthCheck pings the server health endpoint.
func (c *Client) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("sync client: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync client: health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync client: health check returned %d", resp.StatusCode)
	}
	return nil
}

// post sends a POST request and handles common error responses.
func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sync client: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("sync client: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sync client: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sync client: read body: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return body, nil
	case http.StatusUnauthorized:
		return nil, ClassifyAuthError(body, http.StatusUnauthorized)
	case http.StatusForbidden:
		return nil, ClassifyAuthError(body, http.StatusForbidden)
	default:
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
			return nil, &errResp
		}
		return nil, fmt.Errorf("sync client: server returned %d: %s", resp.StatusCode, string(body))
	}
}

// ClassifyAuthError inspects the server's `{"error": "..."}` body to map a
// 401/403 response to the most accurate sentinel. The trasker server emits
// distinct error strings for distinct failure modes (see
// internal/server/auth/apikey.go) and the client must not collapse them all
// into "API key expired" — that lies to the user.
//
// Mapping (case-insensitive substring match on the body's `error` field):
//   - "expired"        → ErrKeyExpired
//   - "revoked"        → ErrKeyRevoked
//   - any other 401    → ErrKeyInvalid    (missing/invalid header, bad key, bcrypt mismatch)
//   - any other 403    → ErrPermissionDenied
//
// If the body is empty or not JSON-parseable, fall back to the conservative
// default for the status code (ErrKeyInvalid for 401, ErrPermissionDenied
// for 403). The previous behavior — always returning ErrKeyExpired on 401 —
// is intentionally NOT preserved.
//
// Exported because the layout syncer in internal/client/layout/sync.go
// performs its own HTTP POST and reuses this classification (single source
// of truth for auth-error mapping, matches the project's "shared sentinel
// set" pattern referenced in layout/sync.go).
func ClassifyAuthError(body []byte, status int) error {
	var parsed serverErrorBody
	_ = json.Unmarshal(body, &parsed)
	msg := strings.ToLower(parsed.Error)

	if strings.Contains(msg, "expired") {
		return ErrKeyExpired
	}
	if strings.Contains(msg, "revoked") {
		return ErrKeyRevoked
	}
	if status == http.StatusForbidden {
		return ErrPermissionDenied
	}
	return ErrKeyInvalid
}
