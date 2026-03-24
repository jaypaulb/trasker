// internal/client/store/notes.go
package store

import (
	"fmt"
	"time"
)

// Note represents a row in the notes table.
type Note struct {
	ID             int64
	AnchorEvent    int64
	Text           string
	CreatedAt      time.Time
	CascadeApplied bool
}

// CreateNote inserts a new note for the given focus event and returns its ID.
func (s *Store) CreateNote(anchorEventID int64, text string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO notes (anchor_event, text, created_at) VALUES (?, ?, ?)`,
		anchorEventID, text, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create note: %w", err)
	}
	return result.LastInsertId()
}

// ListNotesForEvent returns all notes attached to a focus event, ordered by creation time.
func (s *Store) ListNotesForEvent(eventID int64) ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT id, anchor_event, text, created_at, cascade_applied
		 FROM notes WHERE anchor_event = ? ORDER BY created_at ASC`, eventID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notes for event %d: %w", eventID, err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		var createdAt string
		var cascadeApplied int
		if err := rows.Scan(&n.ID, &n.AnchorEvent, &n.Text, &createdAt, &cascadeApplied); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.CascadeApplied = cascadeApplied != 0
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// MarkNoteCascadeApplied sets cascade_applied=1 for the given note.
func (s *Store) MarkNoteCascadeApplied(noteID int64) error {
	_, err := s.db.Exec(`UPDATE notes SET cascade_applied = 1 WHERE id = ?`, noteID)
	if err != nil {
		return fmt.Errorf("mark cascade applied for note %d: %w", noteID, err)
	}
	return nil
}
