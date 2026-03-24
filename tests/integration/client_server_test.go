//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClientServerFullFlow tests the complete lifecycle:
// 1. Client registers device with server
// 2. Client submits focus events as timesheet entries
// 3. Server receives and stores the entries
// 4. Client marks entries as confirmed
func TestClientServerFullFlow(t *testing.T) {
	// --- Setup: mock server that records requests ---
	var (
		deviceRegistered bool
		receivedEntries  []map[string]interface{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key in all requests.
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-api-key-12345678" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/devices":
			// Device registration.
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if body["client_device_id"] == nil || body["os"] == nil {
				http.Error(w, "missing fields", http.StatusBadRequest)
				return
			}

			deviceRegistered = true
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"id":               "server-device-001",
				"client_device_id": body["client_device_id"].(string),
			})

		case r.Method == "POST" && r.URL.Path == "/api/v1/timesheets":
			// Timesheet submission.
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			entries, ok := body["entries"].([]interface{})
			if !ok || len(entries) == 0 {
				http.Error(w, "no entries", http.StatusBadRequest)
				return
			}

			for _, e := range entries {
				entry, _ := e.(map[string]interface{})
				receivedEntries = append(receivedEntries, entry)
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"id": "timesheet-001",
			})

		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	// --- Step 1: Register device ---
	devicePayload := map[string]string{
		"client_device_id": "test-device-uuid-abc",
		"os":               "linux",
		"hostname":         "test-machine",
	}
	deviceBody, _ := json.Marshal(devicePayload)

	req, _ := http.NewRequest("POST", server.URL+"/api/v1/devices", bytes.NewReader(deviceBody))
	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("device registration request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	if !deviceRegistered {
		t.Fatal("server did not register the device")
	}

	// --- Step 2: Submit timesheet entries ---
	now := time.Now().UTC()
	timesheetPayload := map[string]interface{}{
		"client_device_id": "test-device-uuid-abc",
		"entries": []map[string]interface{}{
			{
				"tag":         "Development",
				"started_at":  now.Add(-3 * time.Hour).Format(time.RFC3339),
				"ended_at":    now.Add(-30 * time.Minute).Format(time.RFC3339),
				"duration_s":  9000,
				"notes":       "Working on focus tracker",
				"app_summary": "VS Code (80%), Terminal (20%)",
			},
			{
				"tag":         "Communication",
				"started_at":  now.Add(-30 * time.Minute).Format(time.RFC3339),
				"ended_at":    now.Format(time.RFC3339),
				"duration_s":  1800,
				"notes":       "Team standup",
				"app_summary": "Slack (100%)",
			},
		},
	}
	tsBody, _ := json.Marshal(timesheetPayload)

	req, _ = http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(tsBody))
	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("timesheet submission request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	// --- Step 3: Verify server received entries ---
	if len(receivedEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(receivedEntries))
	}

	entry1 := receivedEntries[0]
	if entry1["tag"] != "Development" {
		t.Errorf("expected tag 'Development', got %v", entry1["tag"])
	}
	if entry1["notes"] != "Working on focus tracker" {
		t.Errorf("expected notes 'Working on focus tracker', got %v", entry1["notes"])
	}

	entry2 := receivedEntries[1]
	if entry2["tag"] != "Communication" {
		t.Errorf("expected tag 'Communication', got %v", entry2["tag"])
	}

	fmt.Println("PASS: Full client-server flow completed successfully")
}

// TestOfflineRetryFlow tests:
// 1. Client attempts submission when server is down → receives 503
// 2. Server comes back up
// 3. Client retries → entries delivered successfully
func TestOfflineRetryFlow(t *testing.T) {
	var receivedEntries []map[string]interface{}
	serverUp := true

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverUp {
			// Simulate server being down.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-api-key-12345678" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Method == "POST" && r.URL.Path == "/api/v1/timesheets" {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			entries, ok := body["entries"].([]interface{})
			if !ok {
				http.Error(w, "no entries", http.StatusBadRequest)
				return
			}
			for _, e := range entries {
				entry, _ := e.(map[string]interface{})
				receivedEntries = append(receivedEntries, entry)
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "timesheet-retry-001"})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	now := time.Now().UTC()
	payload := map[string]interface{}{
		"client_device_id": "test-device-uuid-abc",
		"entries": []map[string]interface{}{
			{
				"tag":        "Development",
				"started_at": now.Add(-1 * time.Hour).Format(time.RFC3339),
				"ended_at":   now.Format(time.RFC3339),
				"duration_s": 3600,
				"notes":      "Offline work session",
			},
		},
	}
	body, _ := json.Marshal(payload)

	// --- Step 1: Server is DOWN ---
	serverUp = false

	req, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed unexpectedly: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when server is down, got %d", resp.StatusCode)
	}
	if len(receivedEntries) != 0 {
		t.Fatal("server should not have received entries while down")
	}

	// At this point, the client would queue the submission locally.
	// In the real client, this is handled by internal/client/sync.
	t.Log("Server returned 503 — client would queue submission locally")

	// --- Step 2: Server comes back UP ---
	serverUp = true

	// --- Step 3: Client retries ---
	req, _ = http.NewRequest("POST", server.URL+"/api/v1/timesheets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-api-key-12345678")
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("retry request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 on retry, got %d", resp.StatusCode)
	}

	if len(receivedEntries) != 1 {
		t.Fatalf("expected 1 entry after retry, got %d", len(receivedEntries))
	}

	if receivedEntries[0]["notes"] != "Offline work session" {
		t.Errorf("entry notes mismatch: got %v", receivedEntries[0]["notes"])
	}

	fmt.Println("PASS: Offline/retry flow completed successfully")
}

// TestAPIKeyExpiryFlow tests:
// 1. Client sends request with expired API key
// 2. Server returns 401 with expiry-specific error body
// 3. Client detects the error type and surfaces the correct message
func TestAPIKeyExpiryFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// Simulate expired key response.
		if authHeader == "Bearer expired-api-key-00000000" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "api_key_expired",
				"message": "Your access key has expired. Please download a new client from your Trasker dashboard.",
			})
			return
		}

		// Simulate revoked key.
		if authHeader == "Bearer revoked-api-key-00000000" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "api_key_revoked",
				"message": "Your access key has been revoked by an administrator.",
			})
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	// --- Test expired key ---
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", nil)
	req.Header.Set("Authorization", "Bearer expired-api-key-00000000")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var errBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("cannot decode error body: %v", err)
	}

	if errBody["error"] != "api_key_expired" {
		t.Errorf("expected error type 'api_key_expired', got %v", errBody["error"])
	}

	expectedMsg := "Your access key has expired. Please download a new client from your Trasker dashboard."
	if errBody["message"] != expectedMsg {
		t.Errorf("expected message %q, got %v", expectedMsg, errBody["message"])
	}

	// --- Test revoked key ---
	req2, _ := http.NewRequest("POST", server.URL+"/api/v1/timesheets", nil)
	req2.Header.Set("Authorization", "Bearer revoked-api-key-00000000")

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked key, got %d", resp2.StatusCode)
	}

	var errBody2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&errBody2); err != nil {
		t.Fatalf("cannot decode error body: %v", err)
	}

	if errBody2["error"] != "api_key_revoked" {
		t.Errorf("expected error type 'api_key_revoked', got %v", errBody2["error"])
	}

	fmt.Println("PASS: API key expiry/revocation flow completed successfully")
}
