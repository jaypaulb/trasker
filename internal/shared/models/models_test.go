package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeviceRegistrationRequestJSON(t *testing.T) {
	req := DeviceRegistrationRequest{
		ClientDeviceID: "abc-123",
		OS:             "linux",
		Hostname:       "dev-machine",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got DeviceRegistrationRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != req {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, req)
	}

	// Verify JSON field names
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["client_device_id"]; !ok {
		t.Error("expected JSON key 'client_device_id', not found")
	}
	if _, ok := raw["os"]; !ok {
		t.Error("expected JSON key 'os', not found")
	}
	if _, ok := raw["hostname"]; !ok {
		t.Error("expected JSON key 'hostname', not found")
	}
}

func TestDeviceRegistrationResponseJSON(t *testing.T) {
	resp := DeviceRegistrationResponse{
		DeviceID:       "server-uuid-456",
		ClientDeviceID: "abc-123",
		Status:         "registered",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["device_id"]; !ok {
		t.Error("expected JSON key 'device_id', not found")
	}
	if _, ok := raw["status"]; !ok {
		t.Error("expected JSON key 'status', not found")
	}
}

func TestTimesheetSubmissionRequestJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	end := now.Add(3 * time.Hour)

	req := TimesheetSubmissionRequest{
		ClientDeviceID: "abc-123",
		Entries: []TimesheetEntry{
			{
				Tag:        "Development",
				StartedAt:  now,
				EndedAt:    end,
				DurationS:  10800,
				Notes:      "Worked on auth",
				AppSummary: "VS Code (80%), Terminal (20%)",
			},
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got TimesheetSubmissionRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ClientDeviceID != req.ClientDeviceID {
		t.Errorf("ClientDeviceID: got %q, want %q", got.ClientDeviceID, req.ClientDeviceID)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries count: got %d, want 1", len(got.Entries))
	}
	if got.Entries[0].Tag != "Development" {
		t.Errorf("tag: got %q, want %q", got.Entries[0].Tag, "Development")
	}
	if got.Entries[0].DurationS != 10800 {
		t.Errorf("duration_s: got %d, want 10800", got.Entries[0].DurationS)
	}
}

func TestTimesheetSubmissionResponseJSON(t *testing.T) {
	resp := TimesheetSubmissionResponse{
		TimesheetID:  "ts-uuid-789",
		Status:       "accepted",
		EntryCount:   3,
		SubmittedAt:  time.Now().UTC().Truncate(time.Second),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["timesheet_id"]; !ok {
		t.Error("expected JSON key 'timesheet_id', not found")
	}
	if _, ok := raw["entry_count"]; !ok {
		t.Error("expected JSON key 'entry_count', not found")
	}
}

func TestAPIErrorJSON(t *testing.T) {
	e := APIError{
		Code:    422,
		Message: "validation failed",
		Detail:  "client_device_id is required",
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["code"]; !ok {
		t.Error("expected JSON key 'code', not found")
	}
	if _, ok := raw["message"]; !ok {
		t.Error("expected JSON key 'message', not found")
	}
	if _, ok := raw["detail"]; !ok {
		t.Error("expected JSON key 'detail', not found")
	}
}

func TestAPIErrorDetailOmitEmpty(t *testing.T) {
	e := APIError{
		Code:    500,
		Message: "internal error",
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["detail"]; ok {
		t.Error("expected 'detail' to be omitted when empty")
	}
}

func TestAPIErrorImplementsError(t *testing.T) {
	e := APIError{Code: 404, Message: "not found"}
	var err error = &e
	want := "API error 404: not found"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestHealthResponseJSON(t *testing.T) {
	h := HealthResponse{
		Status:  "ok",
		Version: "v1.0.0",
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["status"] != "ok" {
		t.Errorf("status: got %v, want %q", raw["status"], "ok")
	}
	if raw["version"] != "v1.0.0" {
		t.Errorf("version: got %v, want %q", raw["version"], "v1.0.0")
	}
}

func TestDeviceRegistrationRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     DeviceRegistrationRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     DeviceRegistrationRequest{ClientDeviceID: "abc", OS: "linux", Hostname: "host"},
			wantErr: false,
		},
		{
			name:    "missing client_device_id",
			req:     DeviceRegistrationRequest{OS: "linux", Hostname: "host"},
			wantErr: true,
		},
		{
			name:    "missing os",
			req:     DeviceRegistrationRequest{ClientDeviceID: "abc", Hostname: "host"},
			wantErr: true,
		},
		{
			name:    "missing hostname is ok",
			req:     DeviceRegistrationRequest{ClientDeviceID: "abc", OS: "linux"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimesheetSubmissionRequestValidate(t *testing.T) {
	validEntry := TimesheetEntry{
		Tag:       "Dev",
		StartedAt: time.Now().UTC(),
		EndedAt:   time.Now().UTC().Add(time.Hour),
		DurationS: 3600,
	}

	tests := []struct {
		name    string
		req     TimesheetSubmissionRequest
		wantErr bool
	}{
		{
			name:    "valid",
			req:     TimesheetSubmissionRequest{ClientDeviceID: "abc", Entries: []TimesheetEntry{validEntry}},
			wantErr: false,
		},
		{
			name:    "missing client_device_id",
			req:     TimesheetSubmissionRequest{Entries: []TimesheetEntry{validEntry}},
			wantErr: true,
		},
		{
			name:    "empty entries",
			req:     TimesheetSubmissionRequest{ClientDeviceID: "abc", Entries: []TimesheetEntry{}},
			wantErr: true,
		},
		{
			name:    "nil entries",
			req:     TimesheetSubmissionRequest{ClientDeviceID: "abc"},
			wantErr: true,
		},
		{
			name: "entry missing tag",
			req: TimesheetSubmissionRequest{
				ClientDeviceID: "abc",
				Entries: []TimesheetEntry{
					{StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC().Add(time.Hour), DurationS: 3600},
				},
			},
			wantErr: true,
		},
		{
			name: "entry ended_at before started_at",
			req: TimesheetSubmissionRequest{
				ClientDeviceID: "abc",
				Entries: []TimesheetEntry{
					{Tag: "Dev", StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC().Add(-time.Hour), DurationS: 3600},
				},
			},
			wantErr: true,
		},
		{
			name: "entry zero duration",
			req: TimesheetSubmissionRequest{
				ClientDeviceID: "abc",
				Entries: []TimesheetEntry{
					{Tag: "Dev", StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC().Add(time.Hour), DurationS: 0},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
