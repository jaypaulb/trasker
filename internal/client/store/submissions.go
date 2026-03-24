// internal/client/store/submissions.go
package store

import (
	"fmt"
	"time"
)

// CreateSubmission inserts a new submission with status "pending".
// Full submission CRUD is implemented in Task 6.
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
// Full submission CRUD is implemented in Task 6.
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
