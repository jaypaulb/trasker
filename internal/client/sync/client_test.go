// internal/client/sync/client_test.go
package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_RegisterDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/devices" {
			t.Errorf("expected /api/v1/devices, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong auth header")
		}

		var reg DeviceRegistration
		json.NewDecoder(r.Body).Decode(&reg)
		if reg.ClientDeviceID != "dev-123" {
			t.Errorf("expected device id 'dev-123', got %q", reg.ClientDeviceID)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{
		ClientDeviceID: "dev-123",
		OS:             "linux",
		Hostname:       "workstation",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestClient_SubmitTimesheet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/timesheets" {
			t.Errorf("expected /api/v1/timesheets, got %s", r.URL.Path)
		}

		var sub TimesheetSubmission
		json.NewDecoder(r.Body).Decode(&sub)
		if len(sub.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(sub.Entries))
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{
			ID:          "server-id-abc",
			SubmittedAt: "2026-03-23T12:00:00Z",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resp, err := client.SubmitTimesheet(context.Background(), TimesheetSubmission{
		ClientDeviceID: "dev-123",
		Entries: []TimesheetEntry{
			{Tag: "Dev", StartedAt: "2026-03-23T09:00:00Z", EndedAt: "2026-03-23T12:00:00Z", DurationS: 10800},
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if resp.ID != "server-id-abc" {
		t.Errorf("expected id 'server-id-abc', got %q", resp.ID)
	}
}

// authErrorCase drives the table-driven test for ClassifyAuthError /
// post()'s 401/403 branch. Bug-3: the client must NOT collapse every 401
// into ErrKeyExpired — the actual cause comes from the server's
// `{"error": "..."}` body.
type authErrorCase struct {
	name     string
	status   int
	body     string
	wantErr  error
	wantDesc string
}

func TestClient_AuthErrorClassification(t *testing.T) {
	cases := []authErrorCase{
		{
			name:    "401 expired -> ErrKeyExpired",
			status:  http.StatusUnauthorized,
			body:    `{"error":"API key has expired","message":"Please download a new client from your Trasker dashboard"}`,
			wantErr: ErrKeyExpired,
		},
		{
			name:    "401 revoked -> ErrKeyRevoked",
			status:  http.StatusUnauthorized,
			body:    `{"error":"API key has been revoked"}`,
			wantErr: ErrKeyRevoked,
		},
		{
			name:    "401 invalid API key -> ErrKeyInvalid",
			status:  http.StatusUnauthorized,
			body:    `{"error":"invalid API key"}`,
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "401 invalid API key format -> ErrKeyInvalid",
			status:  http.StatusUnauthorized,
			body:    `{"error":"invalid API key format"}`,
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "401 missing Authorization header -> ErrKeyInvalid",
			status:  http.StatusUnauthorized,
			body:    `{"error":"missing Authorization header"}`,
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "401 invalid Authorization header format -> ErrKeyInvalid",
			status:  http.StatusUnauthorized,
			body:    `{"error":"invalid Authorization header format"}`,
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "401 with empty body -> ErrKeyInvalid (conservative default)",
			status:  http.StatusUnauthorized,
			body:    "",
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "401 with non-JSON body -> ErrKeyInvalid",
			status:  http.StatusUnauthorized,
			body:    "not json",
			wantErr: ErrKeyInvalid,
		},
		{
			name:    "403 with empty body -> ErrPermissionDenied",
			status:  http.StatusForbidden,
			body:    "",
			wantErr: ErrPermissionDenied,
		},
		{
			name:    "403 forbidden message -> ErrPermissionDenied",
			status:  http.StatusForbidden,
			body:    `{"error":"forbidden"}`,
			wantErr: ErrPermissionDenied,
		},
		{
			name:    "403 revoked -> ErrKeyRevoked (revoked wins over status)",
			status:  http.StatusForbidden,
			body:    `{"error":"API key has been revoked"}`,
			wantErr: ErrKeyRevoked,
		},
		{
			name:    "case-insensitive: EXPIRED uppercase -> ErrKeyExpired",
			status:  http.StatusUnauthorized,
			body:    `{"error":"EXPIRED"}`,
			wantErr: ErrKeyExpired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-key")
			err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
			if err != tc.wantErr {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestClassifyAuthError_Direct exercises the exported helper directly so the
// layout syncer (which calls ClassifyAuthError without going through
// Client.post) is covered by the same matrix.
func TestClassifyAuthError_Direct(t *testing.T) {
	if got := ClassifyAuthError([]byte(`{"error":"API key has expired"}`), 401); got != ErrKeyExpired {
		t.Errorf("expired: got %v, want ErrKeyExpired", got)
	}
	if got := ClassifyAuthError([]byte(`{"error":"API key has been revoked"}`), 401); got != ErrKeyRevoked {
		t.Errorf("revoked: got %v, want ErrKeyRevoked", got)
	}
	if got := ClassifyAuthError([]byte(`{"error":"invalid API key"}`), 401); got != ErrKeyInvalid {
		t.Errorf("invalid: got %v, want ErrKeyInvalid", got)
	}
	if got := ClassifyAuthError(nil, 403); got != ErrPermissionDenied {
		t.Errorf("403 nil body: got %v, want ErrPermissionDenied", got)
	}
}

func TestClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Code: 500, Message: "internal error"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("expected *ErrorResponse, got %T", err)
	}
	if errResp.Code != 500 {
		t.Errorf("expected code 500, got %d", errResp.Code)
	}
}

func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("expected /api/v1/health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
}
