// internal/client/sync/client.go
package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeviceRegistration is the payload for POST /api/v1/devices.
type DeviceRegistration struct {
	ClientDeviceID string `json:"client_device_id"`
	OS             string `json:"os"`
	Hostname       string `json:"hostname"`
}

// TimesheetEntry is a single entry in a timesheet submission.
type TimesheetEntry struct {
	Tag        string `json:"tag"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	DurationS  int    `json:"duration_s"`
	Notes      string `json:"notes,omitempty"`
	AppSummary string `json:"app_summary,omitempty"`
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

// ErrKeyExpired indicates the API key has expired.
var ErrKeyExpired = fmt.Errorf("API key expired — download a new client from your Trasker dashboard")

// ErrKeyRevoked indicates the API key has been revoked.
var ErrKeyRevoked = fmt.Errorf("API key revoked — contact your administrator")

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
		return nil, ErrKeyExpired
	case http.StatusForbidden:
		return nil, ErrKeyRevoked
	default:
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
			return nil, &errResp
		}
		return nil, fmt.Errorf("sync client: server returned %d: %s", resp.StatusCode, string(body))
	}
}
