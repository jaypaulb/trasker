// internal/client/store/pomodoro.go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// PomodoroSession represents a row in pomodoro_sessions.
type PomodoroSession struct {
	ID        int64
	StartedAt time.Time
	EndedAt   *time.Time
	WorkMins  int
	BreakMins int
	Status    string // "work", "break", "done", "cancelled"
	TagID     *int64
	CreatedAt time.Time
}

// PomodoroStore handles CRUD for pomodoro_sessions.
type PomodoroStore struct {
	db *sql.DB
}

// NewPomodoroStore creates a new pomodoro store.
func NewPomodoroStore(db *sql.DB) *PomodoroStore {
	return &PomodoroStore{db: db}
}

// Create inserts a new pomodoro session. Returns the new ID.
func (s *PomodoroStore) Create(workMins, breakMins int, status string, tagID *int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO pomodoro_sessions (started_at, work_mins, break_mins, status, tag_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		now, workMins, breakMins, status, tagID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("pomodoro store: create: %w", err)
	}
	return result.LastInsertId()
}

// UpdateStatus updates the status of a pomodoro session.
func (s *PomodoroStore) UpdateStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE pomodoro_sessions SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("pomodoro store: update status: %w", err)
	}
	return nil
}

// End marks a session as ended with the given status.
func (s *PomodoroStore) End(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE pomodoro_sessions SET ended_at = ?, status = ? WHERE id = ?`,
		now, status, id,
	)
	if err != nil {
		return fmt.Errorf("pomodoro store: end: %w", err)
	}
	return nil
}

// GetByID fetches a single pomodoro session.
func (s *PomodoroStore) GetByID(id int64) (*PomodoroSession, error) {
	row := s.db.QueryRow(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions WHERE id = ?`, id,
	)
	return scanPomodoroSession(row)
}

// ListByDate returns pomodoro sessions for a given date (UTC).
func (s *PomodoroStore) ListByDate(date time.Time) ([]PomodoroSession, error) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	dayEnd := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC).Format(time.RFC3339)

	rows, err := s.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions
		 WHERE started_at >= ? AND started_at <= ?
		 ORDER BY started_at DESC`, dayStart, dayEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("pomodoro store: list by date: %w", err)
	}
	defer rows.Close()

	var sessions []PomodoroSession
	for rows.Next() {
		ps, err := scanPomodoroSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, ps)
	}
	return sessions, rows.Err()
}

// ListRecent returns the N most recent pomodoro sessions.
func (s *PomodoroStore) ListRecent(limit int) ([]PomodoroSession, error) {
	rows, err := s.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions
		 ORDER BY started_at DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pomodoro store: list recent: %w", err)
	}
	defer rows.Close()

	var sessions []PomodoroSession
	for rows.Next() {
		ps, err := scanPomodoroSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, ps)
	}
	return sessions, rows.Err()
}

type pomodoroScanner interface {
	Scan(dest ...any) error
}

func scanPomodoroFromScanner(s pomodoroScanner) (PomodoroSession, error) {
	var ps PomodoroSession
	var startedAt, createdAt string
	var endedAt sql.NullString
	var tagID sql.NullInt64

	err := s.Scan(&ps.ID, &startedAt, &endedAt, &ps.WorkMins, &ps.BreakMins,
		&ps.Status, &tagID, &createdAt)
	if err != nil {
		return ps, fmt.Errorf("pomodoro store: scan: %w", err)
	}

	ps.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	ps.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		ps.EndedAt = &t
	}
	if tagID.Valid {
		v := tagID.Int64
		ps.TagID = &v
	}
	return ps, nil
}

func scanPomodoroSession(row *sql.Row) (*PomodoroSession, error) {
	ps, err := scanPomodoroFromScanner(row)
	if err != nil {
		return nil, err
	}
	return &ps, nil
}

func scanPomodoroSessionRow(rows *sql.Rows) (PomodoroSession, error) {
	return scanPomodoroFromScanner(rows)
}
