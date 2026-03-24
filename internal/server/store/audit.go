// internal/server/store/audit.go
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditLogEntry represents a row in the audit_log table.
type AuditLogEntry struct {
	ID         uuid.UUID
	AdminID    uuid.UUID
	Action     string
	TargetType string
	TargetID   uuid.UUID
	OldValue   json.RawMessage
	NewValue   json.RawMessage
	CreatedAt  time.Time
}

// CreateAuditLogParams holds parameters for creating an audit log entry.
type CreateAuditLogParams struct {
	AdminID    uuid.UUID
	Action     string
	TargetType string
	TargetID   uuid.UUID
	OldValue   json.RawMessage // nullable
	NewValue   json.RawMessage // nullable
}

// CreateAuditLog inserts a new audit log entry.
func (s *Store) CreateAuditLog(ctx context.Context, p CreateAuditLogParams) (*AuditLogEntry, error) {
	var e AuditLogEntry
	err := s.pool.QueryRow(ctx,
		`INSERT INTO audit_log (admin_id, action, target_type, target_id, old_value, new_value)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, admin_id, action, target_type, target_id, old_value, new_value, created_at`,
		p.AdminID, p.Action, p.TargetType, p.TargetID, p.OldValue, p.NewValue,
	).Scan(&e.ID, &e.AdminID, &e.Action, &e.TargetType, &e.TargetID, &e.OldValue, &e.NewValue, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating audit log: %w", err)
	}
	return &e, nil
}

// AuditLogFilters holds optional filters for listing audit logs.
type AuditLogFilters struct {
	AdminID    *uuid.UUID
	Action     *string
	TargetType *string
	TargetID   *uuid.UUID
}

// ListAuditLogs returns audit log entries matching the given filters.
func (s *Store) ListAuditLogs(ctx context.Context, filters AuditLogFilters) ([]AuditLogEntry, error) {
	query := `SELECT id, admin_id, action, target_type, target_id, old_value, new_value, created_at FROM audit_log WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filters.AdminID != nil {
		query += fmt.Sprintf(" AND admin_id = $%d", argIdx)
		args = append(args, *filters.AdminID)
		argIdx++
	}
	if filters.Action != nil {
		query += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, *filters.Action)
		argIdx++
	}
	if filters.TargetType != nil {
		query += fmt.Sprintf(" AND target_type = $%d", argIdx)
		args = append(args, *filters.TargetType)
		argIdx++
	}
	if filters.TargetID != nil {
		query += fmt.Sprintf(" AND target_id = $%d", argIdx)
		args = append(args, *filters.TargetID)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing audit logs: %w", err)
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.AdminID, &e.Action, &e.TargetType, &e.TargetID, &e.OldValue, &e.NewValue, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning audit log: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
