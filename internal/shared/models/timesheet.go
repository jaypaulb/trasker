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
