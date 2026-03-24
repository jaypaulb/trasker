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

func TestClient_KeyExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "expired-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err != ErrKeyExpired {
		t.Errorf("expected ErrKeyExpired, got %v", err)
	}
}

func TestClient_KeyRevoked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "revoked-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
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
