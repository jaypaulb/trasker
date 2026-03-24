// internal/client/store/submissions.go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Submission represents a row in the submissions table.
type Submission struct {
	ID          int64
	ServerID    *string
	SubmittedAt time.Time
	Status      string
	RetryCount  int
	LastRetry   *time.Time
}

// CreateSubmission inserts a new submission with status "pending".
func (s *Store) CreateSubmission(submittedAt time.Time) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO submissions (submitted_at, status) VALUES (?, ?)`,
		submittedAt.Format(time.RFC3339), "pending",
	)
	if err != nil {
		return 0, fmt.Errorf("create submission: %w", err)
	}
	return result.LastInsertId()
}

// LinkEventToSubmission links a focus event to a submission.
func (s *Store) LinkEventToSubmission(submissionID, eventID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO submission_events (submission_id, event_id) VALUES (?, ?)`,
		submissionID, eventID,
	)
	if err != nil {
		return fmt.Errorf("link event %d to submission %d: %w", eventID, submissionID, err)
	}
	return nil
}

// GetSubmission returns a submission by ID.
func (s *Store) GetSubmission(id int64) (*Submission, error) {
	var sub Submission
	var submittedAt string
	var serverID sql.NullString
	var lastRetry sql.NullString

	err := s.db.QueryRow(
		`SELECT id, server_id, submitted_at, status, retry_count, last_retry
		 FROM submissions WHERE id = ?`, id,
	).Scan(&sub.ID, &serverID, &submittedAt, &sub.Status, &sub.RetryCount, &lastRetry)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("submission %d not found", id)
		}
		return nil, fmt.Errorf("get submission: %w", err)
	}

	sub.SubmittedAt, _ = time.Parse(time.RFC3339, submittedAt)
	if serverID.Valid {
		sub.ServerID = &serverID.String
	}
	if lastRetry.Valid {
		t, _ := time.Parse(time.RFC3339, lastRetry.String)
		sub.LastRetry = &t
	}

	return &sub, nil
}

// UpdateSubmissionStatus updates the status and optionally the server_id.
func (s *Store) UpdateSubmissionStatus(id int64, status, serverID string) error {
	var sid *string
	if serverID != "" {
		sid = &serverID
	}
	_, err := s.db.Exec(
		`UPDATE submissions SET status = ?, server_id = ? WHERE id = ?`,
		status, sid, id,
	)
	if err != nil {
		return fmt.Errorf("update submission %d status: %w", id, err)
	}
	return nil
}

// ListPendingSubmissions returns all submissions with status "pending".
func (s *Store) ListPendingSubmissions() ([]Submission, error) {
	rows, err := s.db.Query(
		`SELECT id, server_id, submitted_at, status, retry_count, last_retry
		 FROM submissions WHERE status = 'pending' ORDER BY submitted_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending submissions: %w", err)
	}
	defer rows.Close()

	var subs []Submission
	for rows.Next() {
		var sub Submission
		var submittedAt string
		var serverID sql.NullString
		var lastRetry sql.NullString

		if err := rows.Scan(&sub.ID, &serverID, &submittedAt, &sub.Status, &sub.RetryCount, &lastRetry); err != nil {
			return nil, fmt.Errorf("scan submission: %w", err)
		}

		sub.SubmittedAt, _ = time.Parse(time.RFC3339, submittedAt)
		if serverID.Valid {
			sub.ServerID = &serverID.String
		}
		if lastRetry.Valid {
			t, _ := time.Parse(time.RFC3339, lastRetry.String)
			sub.LastRetry = &t
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// GetSubmittedEventIDs returns all event IDs that are linked to any submission.
func (s *Store) GetSubmittedEventIDs() ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT event_id FROM submission_events ORDER BY event_id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get submitted event IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan event id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// IncrementSubmissionRetry increments the retry_count and sets last_retry to now.
func (s *Store) IncrementSubmissionRetry(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE submissions SET retry_count = retry_count + 1, last_retry = ? WHERE id = ?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("increment retry for submission %d: %w", id, err)
	}
	return nil
}
