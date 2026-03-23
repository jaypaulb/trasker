// internal/server/store/reports.go
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReportSummaryRow represents one row of a time-by-tag summary report.
type ReportSummaryRow struct {
	Tag            string
	TotalDurationS int
	EntryCount     int
}

// ReportFilters holds filters for report queries.
type ReportFilters struct {
	UserID *uuid.UUID
	From   *time.Time
	To     *time.Time
}

// GetReportSummary returns time summed by tag, with optional user and date filters.
func (s *Store) GetReportSummary(ctx context.Context, filters ReportFilters) ([]ReportSummaryRow, error) {
	query := `SELECT te.tag, SUM(te.duration_s) AS total_duration_s, COUNT(*) AS entry_count
		FROM timesheet_entries te
		JOIN timesheets t ON te.timesheet_id = t.id
		WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filters.UserID != nil {
		query += fmt.Sprintf(" AND t.user_id = $%d", argIdx)
		args = append(args, *filters.UserID)
		argIdx++
	}
	if filters.From != nil {
		query += fmt.Sprintf(" AND te.started_at >= $%d", argIdx)
		args = append(args, *filters.From)
		argIdx++
	}
	if filters.To != nil {
		query += fmt.Sprintf(" AND te.ended_at <= $%d", argIdx)
		args = append(args, *filters.To)
		argIdx++
	}

	query += " GROUP BY te.tag ORDER BY total_duration_s DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying report summary: %w", err)
	}
	defer rows.Close()

	var result []ReportSummaryRow
	for rows.Next() {
		var r ReportSummaryRow
		if err := rows.Scan(&r.Tag, &r.TotalDurationS, &r.EntryCount); err != nil {
			return nil, fmt.Errorf("scanning report row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ReportExportRow represents a flat row for CSV export.
type ReportExportRow struct {
	UserEmail   string
	DisplayName string
	Tag         string
	StartedAt   time.Time
	EndedAt     time.Time
	DurationS   int
	Notes       *string
	AppSummary  *string
	SubmittedAt time.Time
}

// GetReportExport returns flat rows for CSV export.
func (s *Store) GetReportExport(ctx context.Context, filters ReportFilters) ([]ReportExportRow, error) {
	query := `SELECT u.email, u.display_name, te.tag, te.started_at, te.ended_at,
		te.duration_s, te.notes, te.app_summary, t.submitted_at
		FROM timesheet_entries te
		JOIN timesheets t ON te.timesheet_id = t.id
		JOIN users u ON t.user_id = u.id
		WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filters.UserID != nil {
		query += fmt.Sprintf(" AND t.user_id = $%d", argIdx)
		args = append(args, *filters.UserID)
		argIdx++
	}
	if filters.From != nil {
		query += fmt.Sprintf(" AND te.started_at >= $%d", argIdx)
		args = append(args, *filters.From)
		argIdx++
	}
	if filters.To != nil {
		query += fmt.Sprintf(" AND te.ended_at <= $%d", argIdx)
		args = append(args, *filters.To)
		argIdx++
	}

	query += " ORDER BY te.started_at"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying report export: %w", err)
	}
	defer rows.Close()

	var result []ReportExportRow
	for rows.Next() {
		var r ReportExportRow
		if err := rows.Scan(&r.UserEmail, &r.DisplayName, &r.Tag, &r.StartedAt, &r.EndedAt,
			&r.DurationS, &r.Notes, &r.AppSummary, &r.SubmittedAt); err != nil {
			return nil, fmt.Errorf("scanning export row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
