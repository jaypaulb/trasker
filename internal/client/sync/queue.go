// internal/client/sync/queue.go
package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"
)

// Backoff schedule for retries: 1m, 5m, 15m, 1h, then hourly.
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
}

// PendingSubmission represents a locally queued submission awaiting sync.
type PendingSubmission struct {
	ID         int64
	Status     string
	RetryCount int
	LastRetry  *time.Time
	CreatedAt  time.Time
}

// Queue processes pending submissions from the local store.
type Queue struct {
	db       *sql.DB
	client   *Client
	deviceID string
	logger   *slog.Logger
	mu       gosync.Mutex
	running  bool
	stopCh   chan struct{}
}

// NewQueue creates a submission queue.
func NewQueue(db *sql.DB, client *Client, deviceID string, logger *slog.Logger) *Queue {
	return &Queue{
		db:       db,
		client:   client,
		deviceID: deviceID,
		logger:   logger,
	}
}

// Start begins periodic processing of the submission queue.
// Processes immediately on start, then every 5 minutes.
func (q *Queue) Start(ctx context.Context) {
	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	q.stopCh = make(chan struct{})
	q.mu.Unlock()

	go q.run(ctx)
}

// Stop halts queue processing.
func (q *Queue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running {
		close(q.stopCh)
		q.running = false
	}
}

func (q *Queue) run(ctx context.Context) {
	// Process immediately
	q.ProcessPending(ctx)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.ProcessPending(ctx)
		}
	}
}

// ProcessPending attempts to submit all pending entries that are due for retry.
func (q *Queue) ProcessPending(ctx context.Context) {
	pending, err := q.listDueSubmissions()
	if err != nil {
		q.logger.Error("failed to list pending submissions", "error", err)
		return
	}

	for _, sub := range pending {
		if ctx.Err() != nil {
			return
		}
		q.processOne(ctx, sub)
	}
}

// listDueSubmissions returns pending submissions that are ready for retry.
func (q *Queue) listDueSubmissions() ([]PendingSubmission, error) {
	rows, err := q.db.Query(
		`SELECT id, status, retry_count, last_retry
		 FROM submissions
		 WHERE status = 'pending'
		 ORDER BY submitted_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: list pending: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var due []PendingSubmission
	for rows.Next() {
		var sub PendingSubmission
		var lastRetry sql.NullString
		if err := rows.Scan(&sub.ID, &sub.Status, &sub.RetryCount, &lastRetry); err != nil {
			return nil, fmt.Errorf("queue: scan: %w", err)
		}
		if lastRetry.Valid {
			t, _ := time.Parse(time.RFC3339, lastRetry.String)
			sub.LastRetry = &t
		}

		// Check if enough time has passed since last retry
		if sub.LastRetry != nil {
			backoff := q.backoffFor(sub.RetryCount)
			if now.Before(sub.LastRetry.Add(backoff)) {
				continue // not due yet
			}
		}

		due = append(due, sub)
	}
	return due, rows.Err()
}

// backoffFor returns the backoff duration for the given retry count.
func (q *Queue) backoffFor(retryCount int) time.Duration {
	if retryCount < len(backoffSchedule) {
		return backoffSchedule[retryCount]
	}
	return 1 * time.Hour // cap at hourly
}

// processOne attempts to submit a single pending submission.
func (q *Queue) processOne(ctx context.Context, sub PendingSubmission) {
	// Load the submission's entries from the store
	entries, err := q.loadSubmissionEntries(sub.ID)
	if err != nil {
		q.logger.Error("failed to load submission entries", "submission_id", sub.ID, "error", err)
		return
	}

	if len(entries) == 0 {
		q.logger.Warn("submission has no entries, marking confirmed", "submission_id", sub.ID)
		q.updateStatus(sub.ID, "confirmed", nil)
		return
	}

	// Aggregate short focus events before submission (per spec)
	entries = Aggregate(entries)

	resp, err := q.client.SubmitTimesheet(ctx, TimesheetSubmission{
		ClientDeviceID: q.deviceID,
		Entries:        entries,
	})
	if err != nil {
		// Handle permanent errors (don't retry)
		if err == ErrKeyExpired || err == ErrKeyRevoked {
			q.logger.Error("permanent sync error", "submission_id", sub.ID, "error", err)
			return
		}

		// Transient error — bump retry
		q.logger.Warn("submission failed, will retry",
			"submission_id", sub.ID,
			"retry_count", sub.RetryCount+1,
			"error", err,
		)
		q.bumpRetry(sub.ID, sub.RetryCount)
		return
	}

	// Success
	q.updateStatus(sub.ID, "confirmed", &resp.ID)
	q.logger.Info("submission confirmed",
		"submission_id", sub.ID,
		"server_id", resp.ID,
	)
}

// loadSubmissionEntries builds TimesheetEntry payloads for a submission.
func (q *Queue) loadSubmissionEntries(submissionID int64) ([]TimesheetEntry, error) {
	rows, err := q.db.Query(
		`SELECT fe.app_name, fe.started_at, fe.ended_at, fe.duration_s,
		        COALESCE(t.name, 'Untagged') as tag,
		        COALESCE(n.text, '') as note
		 FROM submission_events se
		 JOIN focus_events fe ON fe.id = se.event_id
		 LEFT JOIN event_tags et ON et.event_id = fe.id
		 LEFT JOIN tags t ON t.id = et.tag_id
		 LEFT JOIN notes n ON n.anchor_event = fe.id
		 WHERE se.submission_id = ?
		 ORDER BY fe.started_at`, submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: load entries: %w", err)
	}
	defer rows.Close()

	var entries []TimesheetEntry
	for rows.Next() {
		var appName, startedAt, tag, note string
		var endedAt sql.NullString
		var durationS sql.NullInt64

		if err := rows.Scan(&appName, &startedAt, &endedAt, &durationS, &tag, &note); err != nil {
			return nil, fmt.Errorf("queue: scan entry: %w", err)
		}

		end := ""
		if endedAt.Valid {
			end = endedAt.String
		}
		dur := 0
		if durationS.Valid {
			dur = int(durationS.Int64)
		}

		entries = append(entries, TimesheetEntry{
			Tag:       tag,
			StartedAt: startedAt,
			EndedAt:   end,
			DurationS: dur,
			Notes:     note,
		})
	}
	return entries, rows.Err()
}

func (q *Queue) updateStatus(id int64, status string, serverID *string) {
	_, err := q.db.Exec(
		`UPDATE submissions SET status = ?, server_id = ? WHERE id = ?`,
		status, serverID, id,
	)
	if err != nil {
		q.logger.Error("failed to update submission status", "id", id, "error", err)
	}
}

func (q *Queue) bumpRetry(id int64, currentCount int) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := q.db.Exec(
		`UPDATE submissions SET retry_count = ?, last_retry = ? WHERE id = ?`,
		currentCount+1, now, id,
	)
	if err != nil {
		q.logger.Error("failed to bump retry", "id", id, "error", err)
	}
}
