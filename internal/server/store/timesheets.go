// internal/server/store/timesheets.go
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Timesheet represents a submitted timesheet with its entries.
type Timesheet struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	DeviceID    uuid.UUID
	SubmittedAt time.Time
	CreatedAt   time.Time
	Entries     []TimesheetEntry
}

// TimesheetEntry represents a single entry within a timesheet.
type TimesheetEntry struct {
	ID          uuid.UUID
	TimesheetID uuid.UUID
	Tag         string
	StartedAt   time.Time
	EndedAt     time.Time
	DurationS   int
	Notes       *string
	AppSummary  *string
}

// CreateTimesheetParams holds parameters for creating a timesheet with entries.
type CreateTimesheetParams struct {
	UserID      uuid.UUID
	DeviceID    uuid.UUID
	SubmittedAt time.Time
	Entries     []CreateTimesheetEntryParams
}

// CreateTimesheetEntryParams holds parameters for a single timesheet entry.
type CreateTimesheetEntryParams struct {
	Tag        string
	StartedAt  time.Time
	EndedAt    time.Time
	DurationS  int
	Notes      *string
	AppSummary *string
}

// TimesheetFilters holds optional filters for listing timesheets.
type TimesheetFilters struct {
	UserID *uuid.UUID
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}

// CreateTimesheet inserts a timesheet and its entries in a transaction.
func (s *Store) CreateTimesheet(ctx context.Context, p CreateTimesheetParams) (*Timesheet, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var ts Timesheet
	err = tx.QueryRow(ctx,
		`INSERT INTO timesheets (user_id, device_id, submitted_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, device_id, submitted_at, created_at`,
		p.UserID, p.DeviceID, p.SubmittedAt,
	).Scan(&ts.ID, &ts.UserID, &ts.DeviceID, &ts.SubmittedAt, &ts.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("inserting timesheet: %w", err)
	}

	for _, ep := range p.Entries {
		var entry TimesheetEntry
		err = tx.QueryRow(ctx,
			`INSERT INTO timesheet_entries (timesheet_id, tag, started_at, ended_at, duration_s, notes, app_summary)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id, timesheet_id, tag, started_at, ended_at, duration_s, notes, app_summary`,
			ts.ID, ep.Tag, ep.StartedAt, ep.EndedAt, ep.DurationS, ep.Notes, ep.AppSummary,
		).Scan(&entry.ID, &entry.TimesheetID, &entry.Tag, &entry.StartedAt, &entry.EndedAt, &entry.DurationS, &entry.Notes, &entry.AppSummary)
		if err != nil {
			return nil, fmt.Errorf("inserting timesheet entry: %w", err)
		}
		ts.Entries = append(ts.Entries, entry)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &ts, nil
}

// GetTimesheetByID retrieves a timesheet with all its entries.
func (s *Store) GetTimesheetByID(ctx context.Context, id uuid.UUID) (*Timesheet, error) {
	var ts Timesheet
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, device_id, submitted_at, created_at
		 FROM timesheets WHERE id = $1`,
		id,
	).Scan(&ts.ID, &ts.UserID, &ts.DeviceID, &ts.SubmittedAt, &ts.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting timesheet: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, timesheet_id, tag, started_at, ended_at, duration_s, notes, app_summary
		 FROM timesheet_entries WHERE timesheet_id = $1 ORDER BY started_at`,
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("listing timesheet entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e TimesheetEntry
		if err := rows.Scan(&e.ID, &e.TimesheetID, &e.Tag, &e.StartedAt, &e.EndedAt, &e.DurationS, &e.Notes, &e.AppSummary); err != nil {
			return nil, fmt.Errorf("scanning timesheet entry: %w", err)
		}
		ts.Entries = append(ts.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating timesheet entries: %w", err)
	}

	return &ts, nil
}

// ListTimesheetsByUser returns timesheets for a specific user, newest first.
func (s *Store) ListTimesheetsByUser(ctx context.Context, userID uuid.UUID, opts ...TimesheetFilters) ([]Timesheet, error) {
	limit := 100
	offset := 0
	if len(opts) > 0 {
		if opts[0].Limit > 0 {
			limit = opts[0].Limit
		}
		offset = opts[0].Offset
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, device_id, submitted_at, created_at
		 FROM timesheets WHERE user_id = $1 ORDER BY submitted_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing timesheets by user: %w", err)
	}
	defer rows.Close()

	var timesheets []Timesheet
	for rows.Next() {
		var ts Timesheet
		if err := rows.Scan(&ts.ID, &ts.UserID, &ts.DeviceID, &ts.SubmittedAt, &ts.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning timesheet: %w", err)
		}
		timesheets = append(timesheets, ts)
	}
	return timesheets, rows.Err()
}

// ListTimesheetsAll returns all timesheets with optional filters (for manager/admin views).
func (s *Store) ListTimesheetsAll(ctx context.Context, filters TimesheetFilters) ([]Timesheet, error) {
	query := `SELECT id, user_id, device_id, submitted_at, created_at FROM timesheets WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filters.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argIdx)
		args = append(args, *filters.UserID)
		argIdx++
	}
	if filters.From != nil {
		query += fmt.Sprintf(" AND submitted_at >= $%d", argIdx)
		args = append(args, *filters.From)
		argIdx++
	}
	if filters.To != nil {
		query += fmt.Sprintf(" AND submitted_at <= $%d", argIdx)
		args = append(args, *filters.To)
		argIdx++
	}

	query += " ORDER BY submitted_at DESC"

	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, filters.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing all timesheets: %w", err)
	}
	defer rows.Close()

	var timesheets []Timesheet
	for rows.Next() {
		var ts Timesheet
		if err := rows.Scan(&ts.ID, &ts.UserID, &ts.DeviceID, &ts.SubmittedAt, &ts.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning timesheet: %w", err)
		}
		timesheets = append(timesheets, ts)
	}
	return timesheets, rows.Err()
}

// DeleteTimesheetEntry removes a single entry from a timesheet (admin only, audit logged separately).
func (s *Store) DeleteTimesheetEntry(ctx context.Context, entryID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM timesheet_entries WHERE id = $1`,
		entryID,
	)
	if err != nil {
		return fmt.Errorf("deleting timesheet entry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateTimesheetEntryParams holds the fields that can be modified on a timesheet entry.
type UpdateTimesheetEntryParams struct {
	Tag       *string
	StartedAt *time.Time
	EndedAt   *time.Time
	DurationS *int
	Notes     *string
}

// UpdateTimesheetEntry modifies a timesheet entry (admin only, audit logged separately).
func (s *Store) UpdateTimesheetEntry(ctx context.Context, entryID uuid.UUID, p UpdateTimesheetEntryParams) (*TimesheetEntry, error) {
	// Build dynamic update
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if p.Tag != nil {
		setClauses = append(setClauses, fmt.Sprintf("tag = $%d", argIdx))
		args = append(args, *p.Tag)
		argIdx++
	}
	if p.StartedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("started_at = $%d", argIdx))
		args = append(args, *p.StartedAt)
		argIdx++
	}
	if p.EndedAt != nil {
		setClauses = append(setClauses, fmt.Sprintf("ended_at = $%d", argIdx))
		args = append(args, *p.EndedAt)
		argIdx++
	}
	if p.DurationS != nil {
		setClauses = append(setClauses, fmt.Sprintf("duration_s = $%d", argIdx))
		args = append(args, *p.DurationS)
		argIdx++
	}
	if p.Notes != nil {
		setClauses = append(setClauses, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *p.Notes)
		argIdx++
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	query := "UPDATE timesheet_entries SET "
	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, entryID)
	query += " RETURNING id, timesheet_id, tag, started_at, ended_at, duration_s, notes, app_summary"

	var e TimesheetEntry
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&e.ID, &e.TimesheetID, &e.Tag, &e.StartedAt, &e.EndedAt, &e.DurationS, &e.Notes, &e.AppSummary,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("updating timesheet entry: %w", err)
	}
	return &e, nil
}

// GetTimesheetEntryByID retrieves a single timesheet entry.
func (s *Store) GetTimesheetEntryByID(ctx context.Context, id uuid.UUID) (*TimesheetEntry, error) {
	var e TimesheetEntry
	err := s.pool.QueryRow(ctx,
		`SELECT id, timesheet_id, tag, started_at, ended_at, duration_s, notes, app_summary
		 FROM timesheet_entries WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.TimesheetID, &e.Tag, &e.StartedAt, &e.EndedAt, &e.DurationS, &e.Notes, &e.AppSummary)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting timesheet entry: %w", err)
	}
	return &e, nil
}
