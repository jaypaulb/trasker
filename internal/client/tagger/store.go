// internal/client/tagger/store.go
package tagger

import (
	"database/sql"
	"fmt"
	"time"
)

// Store handles persistence for tag rules in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new rule store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new tag rule. Returns the new rule ID.
func (s *Store) Create(tagID int64, appPattern string, titlePattern *string, priority int, suggested bool) (int64, error) {
	suggestedInt := 0
	if suggested {
		suggestedInt = 1
	}
	result, err := s.db.Exec(
		`INSERT INTO tag_rules (tag_id, app_pattern, title_pattern, priority, suggested, hit_count, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		tagID, appPattern, titlePattern, priority, suggestedInt, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("tagger store: create rule: %w", err)
	}
	return result.LastInsertId()
}

// GetByID fetches a single rule.
func (s *Store) GetByID(id int64) (*Rule, error) {
	row := s.db.QueryRow(
		`SELECT r.id, r.tag_id, t.name, r.app_pattern, r.title_pattern,
		        r.priority, r.suggested, r.hit_count
		 FROM tag_rules r
		 JOIN tags t ON t.id = r.tag_id
		 WHERE r.id = ?`, id,
	)
	return scanRule(row)
}

// ListAll returns all rules ordered for evaluation:
// non-suggested title-pattern first (desc priority), then non-suggested app-only,
// then suggested title-pattern, then suggested app-only.
func (s *Store) ListAll() ([]Rule, error) {
	rows, err := s.db.Query(
		`SELECT r.id, r.tag_id, t.name, r.app_pattern, r.title_pattern,
		        r.priority, r.suggested, r.hit_count
		 FROM tag_rules r
		 JOIN tags t ON t.id = r.tag_id
		 ORDER BY r.suggested ASC,
		          CASE WHEN r.title_pattern IS NOT NULL THEN 0 ELSE 1 END ASC,
		          r.priority DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("tagger store: list all: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		r, err := scanRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Update modifies a rule's patterns and priority.
func (s *Store) Update(id int64, appPattern string, titlePattern *string, priority int) error {
	result, err := s.db.Exec(
		`UPDATE tag_rules SET app_pattern = ?, title_pattern = ?, priority = ? WHERE id = ?`,
		appPattern, titlePattern, priority, id,
	)
	if err != nil {
		return fmt.Errorf("tagger store: update rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found", id)
	}
	return nil
}

// Delete removes a rule.
func (s *Store) Delete(id int64) error {
	result, err := s.db.Exec(`DELETE FROM tag_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("tagger store: delete rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found", id)
	}
	return nil
}

// IncrementHitCount bumps the hit_count for a rule.
func (s *Store) IncrementHitCount(id int64) error {
	_, err := s.db.Exec(`UPDATE tag_rules SET hit_count = hit_count + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("tagger store: increment hit_count: %w", err)
	}
	return nil
}

// AcceptSuggested promotes a suggested rule to a confirmed rule.
func (s *Store) AcceptSuggested(id int64) error {
	result, err := s.db.Exec(`UPDATE tag_rules SET suggested = 0 WHERE id = ? AND suggested = 1`, id)
	if err != nil {
		return fmt.Errorf("tagger store: accept suggested: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found or not suggested", id)
	}
	return nil
}

// scanner interface to support both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanRuleFromScanner(s scanner) (Rule, error) {
	var r Rule
	var titlePattern sql.NullString
	var suggested int

	err := s.Scan(&r.ID, &r.TagID, &r.TagName, &r.AppPattern, &titlePattern,
		&r.Priority, &suggested, &r.HitCount)
	if err != nil {
		return r, fmt.Errorf("tagger store: scan rule: %w", err)
	}
	if titlePattern.Valid {
		r.TitlePattern = &titlePattern.String
	}
	r.Suggested = suggested == 1
	return r, nil
}

func scanRule(row *sql.Row) (*Rule, error) {
	r, err := scanRuleFromScanner(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanRuleRow(rows *sql.Rows) (Rule, error) {
	return scanRuleFromScanner(rows)
}
