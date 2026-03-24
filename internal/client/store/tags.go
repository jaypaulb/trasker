// internal/client/store/tags.go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Tag represents a row in the tags table.
type Tag struct {
	ID        int64
	Name      string
	Color     string
	CreatedAt time.Time
}

// CreateTag inserts a new tag and returns its ID.
func (s *Store) CreateTag(name, color string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO tags (name, color, created_at) VALUES (?, ?, ?)`,
		name, color, now,
	)
	if err != nil {
		return 0, fmt.Errorf("create tag: %w", err)
	}
	return result.LastInsertId()
}

// GetTag returns a tag by ID.
func (s *Store) GetTag(id int64) (*Tag, error) {
	var tag Tag
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, name, color, created_at FROM tags WHERE id = ?`, id,
	).Scan(&tag.ID, &tag.Name, &tag.Color, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tag %d not found", id)
		}
		return nil, fmt.Errorf("get tag: %w", err)
	}
	tag.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &tag, nil
}

// ListTags returns all tags ordered by name.
func (s *Store) ListTags() ([]Tag, error) {
	rows, err := s.db.Query(
		`SELECT id, name, color, created_at FROM tags ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var tag Tag
		var createdAt string
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.Color, &createdAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tag.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// UpdateTag updates the name and color of an existing tag.
func (s *Store) UpdateTag(id int64, name, color string) error {
	_, err := s.db.Exec(
		`UPDATE tags SET name = ?, color = ? WHERE id = ?`,
		name, color, id,
	)
	if err != nil {
		return fmt.Errorf("update tag %d: %w", id, err)
	}
	return nil
}

// DeleteTag removes a tag by ID.
func (s *Store) DeleteTag(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tag %d: %w", id, err)
	}
	return nil
}
