// internal/client/store/focus_events.go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// FocusEvent represents a row in the focus_events table.
type FocusEvent struct {
	ID          int64
	AppName     string
	WindowTitle string
	StartedAt   time.Time
	EndedAt     *time.Time
	DurationS   *int64
	IsIdle      bool
	CreatedAt   time.Time
}

// InsertFocusEvent inserts a new focus event and returns its ID.
// The event starts open-ended (no ended_at).
func (s *Store) InsertFocusEvent(appName, windowTitle string, startedAt time.Time) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO focus_events (app_name, window_title, started_at, created_at)
		 VALUES (?, ?, ?, ?)`,
		appName, windowTitle, startedAt.Format(time.RFC3339), now,
	)
	if err != nil {
		return 0, fmt.Errorf("insert focus event: %w", err)
	}
	return result.LastInsertId()
}

// EndFocusEvent sets the ended_at and duration_s for the given event.
func (s *Store) EndFocusEvent(id int64, endedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE focus_events
		 SET ended_at = ?,
		     duration_s = CAST((strftime('%s', ?) - strftime('%s', started_at)) AS INTEGER)
		 WHERE id = ?`,
		endedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("end focus event %d: %w", id, err)
	}
	return nil
}

// GetFocusEvent returns a single focus event by ID.
func (s *Store) GetFocusEvent(id int64) (*FocusEvent, error) {
	row := s.db.QueryRow(
		`SELECT id, app_name, window_title, started_at, ended_at, duration_s, is_idle, created_at
		 FROM focus_events WHERE id = ?`, id,
	)
	return scanFocusEvent(row)
}

// ListFocusEvents returns all focus events whose started_at falls within [from, to].
func (s *Store) ListFocusEvents(from, to time.Time) ([]FocusEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, app_name, window_title, started_at, ended_at, duration_s, is_idle, created_at
		 FROM focus_events
		 WHERE started_at >= ? AND started_at <= ?
		 ORDER BY started_at ASC`,
		from.Format(time.RFC3339), to.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("list focus events: %w", err)
	}
	defer rows.Close()

	var events []FocusEvent
	for rows.Next() {
		ev, err := scanFocusEventRow(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *ev)
	}
	return events, rows.Err()
}

// MarkFocusEventIdle sets is_idle=1 for the given event.
func (s *Store) MarkFocusEventIdle(id int64) error {
	_, err := s.db.Exec(`UPDATE focus_events SET is_idle = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark idle %d: %w", id, err)
	}
	return nil
}

// scanner is an interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanFocusEventFromScanner(sc scanner) (*FocusEvent, error) {
	var ev FocusEvent
	var startedAt, createdAt string
	var endedAt sql.NullString
	var durationS sql.NullInt64
	var isIdle int

	err := sc.Scan(
		&ev.ID, &ev.AppName, &ev.WindowTitle,
		&startedAt, &endedAt, &durationS, &isIdle, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("focus event not found")
		}
		return nil, fmt.Errorf("scan focus event: %w", err)
	}

	ev.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	ev.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ev.IsIdle = isIdle != 0

	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		ev.EndedAt = &t
	}
	if durationS.Valid {
		ev.DurationS = &durationS.Int64
	}

	return &ev, nil
}

func scanFocusEvent(row *sql.Row) (*FocusEvent, error) {
	return scanFocusEventFromScanner(row)
}

func scanFocusEventRow(rows *sql.Rows) (*FocusEvent, error) {
	return scanFocusEventFromScanner(rows)
}
