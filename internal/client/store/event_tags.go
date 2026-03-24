// internal/client/store/event_tags.go
package store

import (
	"database/sql"
	"fmt"
)

// EventTag represents a row in the event_tags table.
type EventTag struct {
	EventID     int64
	TagID       int64
	Source      string
	CascadeFrom *int64
}

// ApplyTagToEvent links a tag to an event. If the link already exists, it is
// silently ignored (INSERT OR IGNORE).
// source is one of "manual", "cascade", "auto_rule".
// cascadeFrom is the anchor event ID (nil for manual/auto_rule).
func (s *Store) ApplyTagToEvent(eventID, tagID int64, source string, cascadeFrom *int64) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO event_tags (event_id, tag_id, source, cascade_from)
		 VALUES (?, ?, ?, ?)`,
		eventID, tagID, source, cascadeFrom,
	)
	if err != nil {
		return fmt.Errorf("apply tag %d to event %d: %w", tagID, eventID, err)
	}
	return nil
}

// ListTagsForEvent returns all tags applied to a given event.
func (s *Store) ListTagsForEvent(eventID int64) ([]EventTag, error) {
	rows, err := s.db.Query(
		`SELECT event_id, tag_id, source, cascade_from
		 FROM event_tags WHERE event_id = ?`, eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tags for event %d: %w", eventID, err)
	}
	defer rows.Close()

	var result []EventTag
	for rows.Next() {
		var et EventTag
		var cascadeFrom sql.NullInt64
		if err := rows.Scan(&et.EventID, &et.TagID, &et.Source, &cascadeFrom); err != nil {
			return nil, fmt.Errorf("scan event tag: %w", err)
		}
		if cascadeFrom.Valid {
			et.CascadeFrom = &cascadeFrom.Int64
		}
		result = append(result, et)
	}
	return result, rows.Err()
}

// ListEventIDsForTag returns all event IDs tagged with the given tag.
func (s *Store) ListEventIDsForTag(tagID int64) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT event_id FROM event_tags WHERE tag_id = ? ORDER BY event_id ASC`, tagID,
	)
	if err != nil {
		return nil, fmt.Errorf("list events for tag %d: %w", tagID, err)
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

// IsEventSubmitted checks whether an event has been linked to any submission.
func (s *Store) IsEventSubmitted(eventID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM submission_events WHERE event_id = ?`, eventID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check submission for event %d: %w", eventID, err)
	}
	return count > 0, nil
}
