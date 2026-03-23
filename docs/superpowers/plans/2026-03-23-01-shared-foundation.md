# Shared Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Initialize Go module with shared packages (models, apikey, version) and build infrastructure

**Architecture:** Single Go module monorepo with cmd/ entrypoints, internal/ packages, and a Makefile for cross-platform builds

**Tech Stack:** Go 1.22+, modernc.org/sqlite, chi router, bcrypt

---

## Task 1: Go Module Init & Project Skeleton

**Files:**
- Create: `go.mod`
- Create: `cmd/trasker-client/main.go`
- Create: `cmd/trasker-server/main.go`

### Steps

- [ ] **1.1** Initialize the Go module:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go mod init github.com/jaypaulb/trasker
  ```
  Expected: `go.mod` created with `module github.com/jaypaulb/trasker`

- [ ] **1.2** Create client entrypoint stub `cmd/trasker-client/main.go`:
  ```go
  package main

  import (
  	"fmt"

  	"github.com/jaypaulb/trasker/internal/shared/version"
  )

  func main() {
  	fmt.Printf("trasker-client %s (commit: %s)\n", version.Version, version.Commit)
  }
  ```

- [ ] **1.3** Create server entrypoint stub `cmd/trasker-server/main.go`:
  ```go
  package main

  import (
  	"fmt"

  	"github.com/jaypaulb/trasker/internal/shared/version"
  )

  func main() {
  	fmt.Printf("trasker-server %s (commit: %s)\n", version.Version, version.Commit)
  }
  ```

- [ ] **1.4** These won't compile yet (version package doesn't exist). That's expected — they'll compile after Task 2. Verify the module file is correct:
  ```bash
  head -3 go.mod
  ```
  Expected output:
  ```
  module github.com/jaypaulb/trasker

  go 1.22
  ```

- [ ] **1.5** Commit:
  ```bash
  git add go.mod cmd/trasker-client/main.go cmd/trasker-server/main.go
  git commit -m "Initialize Go module and create client/server entrypoint stubs"
  ```

---

## Task 2: Version Package

**Files:**
- Create: `internal/shared/version/version.go`
- Create: `internal/shared/version/version_test.go`

### Steps

- [ ] **2.1** Write the test first — `internal/shared/version/version_test.go`:
  ```go
  package version

  import "testing"

  func TestDefaultVersion(t *testing.T) {
  	if Version != "dev" {
  		t.Errorf("expected default Version to be %q, got %q", "dev", Version)
  	}
  }

  func TestDefaultCommit(t *testing.T) {
  	if Commit != "unknown" {
  		t.Errorf("expected default Commit to be %q, got %q", "unknown", Commit)
  	}
  }

  func TestDefaultAPIKey(t *testing.T) {
  	if APIKey != "" {
  		t.Errorf("expected default APIKey to be empty, got %q", APIKey)
  	}
  }

  func TestDefaultServerURL(t *testing.T) {
  	if ServerURL != "" {
  		t.Errorf("expected default ServerURL to be empty, got %q", ServerURL)
  	}
  }

  func TestString(t *testing.T) {
  	want := "dev (unknown)"
  	got := String()
  	if got != want {
  		t.Errorf("String() = %q, want %q", got, want)
  	}
  }
  ```

- [ ] **2.2** Run the test — expect compile failure (package doesn't exist yet):
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/version/
  ```
  Expected: compilation error — no Go files in the package.

- [ ] **2.3** Implement `internal/shared/version/version.go`:
  ```go
  // Package version holds build-time version info injected via ldflags.
  //
  // Build with:
  //   go build -ldflags "-X github.com/jaypaulb/trasker/internal/shared/version.Version=v1.0.0
  //     -X github.com/jaypaulb/trasker/internal/shared/version.Commit=abc1234
  //     -X github.com/jaypaulb/trasker/internal/shared/version.APIKey=key_...
  //     -X github.com/jaypaulb/trasker/internal/shared/version.ServerURL=https://..."
  package version

  import "fmt"

  // These variables are set at build time via -ldflags -X.
  var (
  	// Version is the semantic version (e.g., "v1.0.0"). Default "dev" for local builds.
  	Version = "dev"

  	// Commit is the git commit hash. Default "unknown" for local builds.
  	Commit = "unknown"

  	// APIKey is the pre-baked API key for client binaries. Empty for server builds
  	// and local dev builds. Set by the server build pipeline when generating
  	// downloadable client binaries.
  	APIKey = ""

  	// ServerURL is the pre-baked server URL for client binaries. Empty for server
  	// builds and local dev builds. Set by the server build pipeline.
  	ServerURL = ""
  )

  // String returns a human-readable version string: "v1.0.0 (abc1234)".
  func String() string {
  	return fmt.Sprintf("%s (%s)", Version, Commit)
  }
  ```

- [ ] **2.4** Run the test — expect pass:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/version/
  ```
  Expected output:
  ```
  ok  	github.com/jaypaulb/trasker/internal/shared/version
  ```

- [ ] **2.5** Verify the cmd stubs now compile:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go build ./cmd/trasker-client/
  go build ./cmd/trasker-server/
  ```
  Expected: no errors, binaries produced.

- [ ] **2.6** Run a client stub to verify output:
  ```bash
  ./trasker-client
  ```
  Expected output:
  ```
  trasker-client dev (commit: unknown)
  ```

- [ ] **2.7** Clean up built binaries:
  ```bash
  rm -f trasker-client trasker-server
  ```

- [ ] **2.8** Commit:
  ```bash
  git add internal/shared/version/version.go internal/shared/version/version_test.go
  git commit -m "Add version package with ldflags-settable build vars"
  ```

---

## Task 3: API Models

**Files:**
- Create: `internal/shared/models/device.go`
- Create: `internal/shared/models/timesheet.go`
- Create: `internal/shared/models/common.go`
- Create: `internal/shared/models/models_test.go`

### Steps

- [ ] **3.1** Write the test first — `internal/shared/models/models_test.go`:
  ```go
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
  	// detail should be omitted when empty — but here it's set
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
  ```

- [ ] **3.2** Run the test — expect compile failure:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/models/
  ```
  Expected: compilation error — types not defined yet.

- [ ] **3.3** Implement `internal/shared/models/common.go`:
  ```go
  // Package models defines the shared API request/response types used by both
  // the Trasker client and server.
  package models

  import "fmt"

  // APIError is the standard error response returned by all API endpoints.
  type APIError struct {
  	Code    int    `json:"code"`
  	Message string `json:"message"`
  	Detail  string `json:"detail,omitempty"`
  }

  // Error implements the error interface.
  func (e *APIError) Error() string {
  	return fmt.Sprintf("API error %d: %s", e.Code, e.Message)
  }

  // HealthResponse is returned by GET /api/v1/health.
  type HealthResponse struct {
  	Status  string `json:"status"`
  	Version string `json:"version"`
  }
  ```

- [ ] **3.4** Implement `internal/shared/models/device.go`:
  ```go
  package models

  import "errors"

  // DeviceRegistrationRequest is sent by the client on first run.
  // POST /api/v1/devices
  type DeviceRegistrationRequest struct {
  	ClientDeviceID string `json:"client_device_id"`
  	OS             string `json:"os"`
  	Hostname       string `json:"hostname,omitempty"`
  }

  // Validate checks required fields.
  func (r *DeviceRegistrationRequest) Validate() error {
  	if r.ClientDeviceID == "" {
  		return errors.New("client_device_id is required")
  	}
  	if r.OS == "" {
  		return errors.New("os is required")
  	}
  	return nil
  }

  // DeviceRegistrationResponse is returned by the server after device registration.
  type DeviceRegistrationResponse struct {
  	DeviceID       string `json:"device_id"`
  	ClientDeviceID string `json:"client_device_id"`
  	Status         string `json:"status"`
  }
  ```

- [ ] **3.5** Implement `internal/shared/models/timesheet.go`:
  ```go
  package models

  import (
  	"errors"
  	"fmt"
  	"time"
  )

  // TimesheetEntry represents a single tagged time block within a submission.
  type TimesheetEntry struct {
  	Tag        string    `json:"tag"`
  	StartedAt  time.Time `json:"started_at"`
  	EndedAt    time.Time `json:"ended_at"`
  	DurationS  int       `json:"duration_s"`
  	Notes      string    `json:"notes,omitempty"`
  	AppSummary string    `json:"app_summary,omitempty"`
  }

  // Validate checks required fields and logical constraints for a single entry.
  func (e *TimesheetEntry) Validate() error {
  	if e.Tag == "" {
  		return errors.New("tag is required")
  	}
  	if e.StartedAt.IsZero() {
  		return errors.New("started_at is required")
  	}
  	if e.EndedAt.IsZero() {
  		return errors.New("ended_at is required")
  	}
  	if !e.EndedAt.After(e.StartedAt) {
  		return fmt.Errorf("ended_at (%s) must be after started_at (%s)", e.EndedAt, e.StartedAt)
  	}
  	if e.DurationS <= 0 {
  		return fmt.Errorf("duration_s must be positive, got %d", e.DurationS)
  	}
  	return nil
  }

  // TimesheetSubmissionRequest is sent by the client to submit tagged time blocks.
  // POST /api/v1/timesheets
  type TimesheetSubmissionRequest struct {
  	ClientDeviceID string           `json:"client_device_id"`
  	Entries        []TimesheetEntry `json:"entries"`
  }

  // Validate checks required fields and validates each entry.
  func (r *TimesheetSubmissionRequest) Validate() error {
  	if r.ClientDeviceID == "" {
  		return errors.New("client_device_id is required")
  	}
  	if len(r.Entries) == 0 {
  		return errors.New("entries must not be empty")
  	}
  	for i := range r.Entries {
  		if err := r.Entries[i].Validate(); err != nil {
  			return fmt.Errorf("entry[%d]: %w", i, err)
  		}
  	}
  	return nil
  }

  // TimesheetSubmissionResponse is returned by the server after accepting a submission.
  type TimesheetSubmissionResponse struct {
  	TimesheetID string    `json:"timesheet_id"`
  	Status      string    `json:"status"`
  	EntryCount  int       `json:"entry_count"`
  	SubmittedAt time.Time `json:"submitted_at"`
  }
  ```

- [ ] **3.6** Run the tests — expect all pass:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/models/ -v
  ```
  Expected: all tests PASS.

- [ ] **3.7** Run `go vet` on the package:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go vet ./internal/shared/models/
  ```
  Expected: no output (clean).

- [ ] **3.8** Commit:
  ```bash
  git add internal/shared/models/common.go internal/shared/models/device.go internal/shared/models/timesheet.go internal/shared/models/models_test.go
  git commit -m "Add shared API request/response models with validation"
  ```

---

## Task 4: API Key Package

**Files:**
- Create: `internal/shared/apikey/apikey.go`
- Create: `internal/shared/apikey/apikey_test.go`

### Steps

- [ ] **4.1** Write the test first — `internal/shared/apikey/apikey_test.go`:
  ```go
  package apikey

  import (
  	"strings"
  	"testing"
  	"time"
  )

  func TestGenerate(t *testing.T) {
  	plaintext, hash, prefix, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() error: %v", err)
  	}

  	// Plaintext should be non-empty and have the "tsk_" prefix
  	if plaintext == "" {
  		t.Error("plaintext is empty")
  	}
  	if !strings.HasPrefix(plaintext, "tsk_") {
  		t.Errorf("plaintext should start with 'tsk_', got %q", plaintext[:10])
  	}

  	// Hash should be non-empty and look like bcrypt ($2a$ prefix)
  	if hash == "" {
  		t.Error("hash is empty")
  	}
  	if !strings.HasPrefix(hash, "$2a$") {
  		t.Errorf("hash should be bcrypt, got prefix %q", hash[:4])
  	}

  	// Prefix should be first 8 chars of plaintext
  	if len(prefix) != 8 {
  		t.Errorf("prefix length: got %d, want 8", len(prefix))
  	}
  	if prefix != plaintext[:8] {
  		t.Errorf("prefix should be first 8 chars of plaintext: got %q, want %q", prefix, plaintext[:8])
  	}
  }

  func TestGenerateUniqueness(t *testing.T) {
  	p1, _, _, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() #1 error: %v", err)
  	}
  	p2, _, _, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() #2 error: %v", err)
  	}
  	if p1 == p2 {
  		t.Error("two generated keys should not be identical")
  	}
  }

  func TestHashAndVerify(t *testing.T) {
  	plaintext, _, _, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() error: %v", err)
  	}

  	hash, err := Hash(plaintext)
  	if err != nil {
  		t.Fatalf("Hash() error: %v", err)
  	}

  	if !Verify(plaintext, hash) {
  		t.Error("Verify() should return true for matching key and hash")
  	}
  }

  func TestVerifyWrongKey(t *testing.T) {
  	p1, _, _, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() error: %v", err)
  	}
  	p2, _, _, err := Generate()
  	if err != nil {
  		t.Fatalf("Generate() error: %v", err)
  	}

  	hash, err := Hash(p1)
  	if err != nil {
  		t.Fatalf("Hash() error: %v", err)
  	}

  	if Verify(p2, hash) {
  		t.Error("Verify() should return false for non-matching key")
  	}
  }

  func TestVerifyEmptyInputs(t *testing.T) {
  	if Verify("", "somehash") {
  		t.Error("Verify() should return false for empty key")
  	}
  	if Verify("somekey", "") {
  		t.Error("Verify() should return false for empty hash")
  	}
  }

  func TestPrefix(t *testing.T) {
  	tests := []struct {
  		name string
  		key  string
  		want string
  	}{
  		{name: "normal key", key: "tsk_abcdefghijklmnop", want: "tsk_abcd"},
  		{name: "exactly 8 chars", key: "12345678", want: "12345678"},
  		{name: "short key", key: "abc", want: "abc"},
  		{name: "empty key", key: "", want: ""},
  	}
  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			got := Prefix(tt.key)
  			if got != tt.want {
  				t.Errorf("Prefix(%q) = %q, want %q", tt.key, got, tt.want)
  			}
  		})
  	}
  }

  func TestIsExpired(t *testing.T) {
  	tests := []struct {
  		name       string
  		lastUsed   time.Time
  		expiryDays int
  		want       bool
  	}{
  		{
  			name:       "used yesterday, 60 day expiry",
  			lastUsed:   time.Now().Add(-24 * time.Hour),
  			expiryDays: 60,
  			want:       false,
  		},
  		{
  			name:       "used 61 days ago, 60 day expiry",
  			lastUsed:   time.Now().Add(-61 * 24 * time.Hour),
  			expiryDays: 60,
  			want:       true,
  		},
  		{
  			name:       "used exactly 60 days ago, 60 day expiry",
  			lastUsed:   time.Now().Add(-60 * 24 * time.Hour),
  			expiryDays: 60,
  			want:       false,
  		},
  		{
  			name:       "used just now, 1 day expiry",
  			lastUsed:   time.Now(),
  			expiryDays: 1,
  			want:       false,
  		},
  		{
  			name:       "zero expiry days means never expires",
  			lastUsed:   time.Now().Add(-365 * 24 * time.Hour),
  			expiryDays: 0,
  			want:       false,
  		},
  	}
  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			got := IsExpired(tt.lastUsed, tt.expiryDays)
  			if got != tt.want {
  				t.Errorf("IsExpired(%v, %d) = %v, want %v", tt.lastUsed, tt.expiryDays, got, tt.want)
  			}
  		})
  	}
  }
  ```

- [ ] **4.2** Run the test — expect compile failure:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/apikey/
  ```
  Expected: compilation error — package doesn't exist yet.

- [ ] **4.3** Implement `internal/shared/apikey/apikey.go`:
  ```go
  // Package apikey handles API key generation, hashing, verification, and expiry
  // checks. Keys are generated with a "tsk_" prefix for easy identification.
  // Server stores bcrypt hashes; plaintext only exists in client binaries.
  package apikey

  import (
  	"crypto/rand"
  	"encoding/hex"
  	"fmt"
  	"time"

  	"golang.org/x/crypto/bcrypt"
  )

  const (
  	// keyPrefix is prepended to all generated API keys for identification.
  	keyPrefix = "tsk_"

  	// randomBytes is the number of random bytes used to generate the key body.
  	// 32 bytes = 64 hex chars, giving 256 bits of entropy.
  	randomBytes = 32

  	// bcryptCost is the bcrypt work factor. 12 is a reasonable default for
  	// API key hashing (not user-facing latency-sensitive).
  	bcryptCost = 12

  	// prefixLen is the number of characters stored as the key prefix for
  	// identification in the admin UI.
  	prefixLen = 8
  )

  // Generate creates a new API key and returns the plaintext key, its bcrypt
  // hash, and the prefix (first 8 characters) for storage.
  func Generate() (plaintext, hash, prefix string, err error) {
  	b := make([]byte, randomBytes)
  	if _, err := rand.Read(b); err != nil {
  		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
  	}

  	plaintext = keyPrefix + hex.EncodeToString(b)

  	hash, err = Hash(plaintext)
  	if err != nil {
  		return "", "", "", err
  	}

  	prefix = Prefix(plaintext)
  	return plaintext, hash, prefix, nil
  }

  // Hash returns the bcrypt hash of the given API key.
  func Hash(key string) (string, error) {
  	h, err := bcrypt.GenerateFromPassword([]byte(key), bcryptCost)
  	if err != nil {
  		return "", fmt.Errorf("bcrypt hash: %w", err)
  	}
  	return string(h), nil
  }

  // Verify checks whether the given plaintext key matches the bcrypt hash.
  // Returns false for empty inputs or mismatches.
  func Verify(key, hash string) bool {
  	if key == "" || hash == "" {
  		return false
  	}
  	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)) == nil
  }

  // Prefix returns the first 8 characters of the key for identification.
  // If the key is shorter than 8 characters, the full key is returned.
  func Prefix(key string) string {
  	if len(key) <= prefixLen {
  		return key
  	}
  	return key[:prefixLen]
  }

  // IsExpired returns true if the key has not been used within expiryDays
  // of the current time. If expiryDays is 0, the key never expires.
  func IsExpired(lastUsed time.Time, expiryDays int) bool {
  	if expiryDays <= 0 {
  		return false
  	}
  	expiry := lastUsed.Add(time.Duration(expiryDays) * 24 * time.Hour)
  	return time.Now().After(expiry)
  }
  ```

- [ ] **4.4** Fetch the bcrypt dependency:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go mod tidy
  ```
  Expected: `golang.org/x/crypto` added to `go.mod` and `go.sum`.

- [ ] **4.5** Run the tests — expect all pass:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go test ./internal/shared/apikey/ -v
  ```
  Expected: all tests PASS. Note: bcrypt tests are intentionally slow (~200ms each due to cost=12).

- [ ] **4.6** Run `go vet`:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go vet ./internal/shared/apikey/
  ```
  Expected: no output (clean).

- [ ] **4.7** Commit:
  ```bash
  git add internal/shared/apikey/apikey.go internal/shared/apikey/apikey_test.go go.mod go.sum
  git commit -m "Add API key package with generation, hashing, verification, and expiry"
  ```

---

## Task 5: Makefile

**Files:**
- Create: `Makefile`

### Steps

- [ ] **5.1** Write a test first — verify current binaries build manually before creating Makefile:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  go build -o /dev/null ./cmd/trasker-client/
  go build -o /dev/null ./cmd/trasker-server/
  ```
  Expected: both compile with no errors.

- [ ] **5.2** Create `Makefile`:
  ```makefile
  # Trasker Makefile
  # Builds client and server binaries for all supported platforms.
  # Version info is stamped via ldflags.

  MODULE := github.com/jaypaulb/trasker
  VERSION_PKG := $(MODULE)/internal/shared/version

  # Version defaults — override with: make VERSION=v1.0.0 COMMIT=abc1234
  VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
  COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

  # ldflags for version stamping
  LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT)

  # Output directory
  DIST := dist

  # Build targets: os/arch pairs
  CLIENT_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
  SERVER_TARGETS := linux/amd64 linux/arm64

  .PHONY: all clean test vet client server client-all server-all

  ## Default: build client and server for the current platform
  all: client server

  ## Build client for current platform
  client:
  	go build -ldflags '$(LDFLAGS)' -o $(DIST)/trasker-client ./cmd/trasker-client/

  ## Build server for current platform
  server:
  	go build -ldflags '$(LDFLAGS)' -o $(DIST)/trasker-server ./cmd/trasker-server/

  ## Build client for all platforms
  client-all:
  	@for target in $(CLIENT_TARGETS); do \
  		os=$${target%/*}; \
  		arch=$${target#*/}; \
  		ext=""; \
  		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
  		echo "Building client: $$os/$$arch"; \
  		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' \
  			-o $(DIST)/trasker-client-$$os-$$arch$$ext ./cmd/trasker-client/ || exit 1; \
  	done

  ## Build server for all platforms
  server-all:
  	@for target in $(SERVER_TARGETS); do \
  		os=$${target%/*}; \
  		arch=$${target#*/}; \
  		echo "Building server: $$os/$$arch"; \
  		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' \
  			-o $(DIST)/trasker-server-$$os-$$arch ./cmd/trasker-server/ || exit 1; \
  	done

  ## Build client with baked-in API key and server URL (used by server build pipeline)
  ## Usage: make client-stamped API_KEY=tsk_... SERVER_URL=https://... GOOS=linux GOARCH=amd64
  client-stamped:
  ifndef API_KEY
  	$(error API_KEY is required for client-stamped)
  endif
  ifndef SERVER_URL
  	$(error SERVER_URL is required for client-stamped)
  endif
  	GOOS=$(or $(GOOS),linux) GOARCH=$(or $(GOARCH),amd64) go build \
  		-ldflags '$(LDFLAGS) -X $(VERSION_PKG).APIKey=$(API_KEY) -X $(VERSION_PKG).ServerURL=$(SERVER_URL)' \
  		-o $(DIST)/trasker-client-$(GOOS)-$(GOARCH) ./cmd/trasker-client/

  ## Run all tests
  test:
  	go test ./...

  ## Run go vet
  vet:
  	go vet ./...

  ## Remove build artifacts
  clean:
  	rm -rf $(DIST)

  ## Show version info that would be stamped
  version:
  	@echo "Version: $(VERSION)"
  	@echo "Commit:  $(COMMIT)"
  ```

- [ ] **5.3** Test `make test`:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  make test
  ```
  Expected: all tests pass across all packages.

- [ ] **5.4** Test `make client server`:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  make client server
  ```
  Expected: `dist/trasker-client` and `dist/trasker-server` created.

- [ ] **5.5** Verify version stamping:
  ```bash
  ./dist/trasker-client
  ```
  Expected output (commit hash will vary):
  ```
  trasker-client <git-describe> (commit: <short-hash>)
  ```

- [ ] **5.6** Test `make clean`:
  ```bash
  cd /home/jaypaulb/Projects/gh/trasker
  make clean && ls dist 2>&1
  ```
  Expected: `dist` directory removed, `ls` shows error.

- [ ] **5.7** Add `dist/` to `.gitignore`:
  ```
  # Ignore all dotfiles by default
  .*

  # Except standard version control files
  !.git
  !.gitignore
  !.gitattributes
  !.gitmodules
  !.gitkeep

  # Build output
  dist/
  ```

- [ ] **5.8** Commit:
  ```bash
  git add Makefile .gitignore
  git commit -m "Add Makefile with cross-platform build targets and version stamping"
  ```

---

## Verification Checkpoint

After all 5 tasks, run the full suite to confirm everything works together:

```bash
cd /home/jaypaulb/Projects/gh/trasker
make test && make vet && make client server && ./dist/trasker-client && ./dist/trasker-server && make clean
```

Expected:
1. All tests pass
2. `go vet` clean
3. Both binaries build
4. Client prints: `trasker-client <version> (commit: <hash>)`
5. Server prints: `trasker-server <version> (commit: <hash>)`
6. `dist/` cleaned up

### Files Created

```
trasker/
├── .gitignore
├── Makefile
├── go.mod
├── go.sum (after go mod tidy)
├── cmd/
│   ├── trasker-client/
│   │   └── main.go
│   └── trasker-server/
│       └── main.go
└── internal/
    └── shared/
        ├── apikey/
        │   ├── apikey.go
        │   └── apikey_test.go
        ├── models/
        │   ├── common.go
        │   ├── device.go
        │   ├── timesheet.go
        │   └── models_test.go
        └── version/
            ├── version.go
            └── version_test.go
```

### Ready for Phase 2

With this foundation in place, **Plan 02 (Server Core)** and **Plan 03 (Client Core)** can proceed in parallel. Both import from `internal/shared/` but write to separate directories (`internal/server/` and `internal/client/`).
