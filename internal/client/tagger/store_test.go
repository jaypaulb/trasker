// internal/client/tagger/store_test.go
package tagger

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create minimal schema needed for rule store
	for _, stmt := range []string{
		`CREATE TABLE tags (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			color TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE tag_rules (
			id INTEGER PRIMARY KEY,
			tag_id INTEGER NOT NULL REFERENCES tags(id),
			app_pattern TEXT NOT NULL,
			title_pattern TEXT,
			priority INTEGER NOT NULL DEFAULT 0,
			suggested INTEGER NOT NULL DEFAULT 0,
			hit_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Development', '#00ff00', '2026-01-01T00:00:00Z')`,
		`INSERT INTO tags (id, name, color, created_at) VALUES (2, 'Communication', '#0000ff', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func TestStore_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	titlePat := "*claude*"
	id, err := store.Create(1, "terminal", &titlePat, 10, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rule, err := store.GetByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rule.TagID != 1 {
		t.Errorf("expected TagID 1, got %d", rule.TagID)
	}
	if rule.AppPattern != "terminal" {
		t.Errorf("expected app_pattern 'terminal', got %q", rule.AppPattern)
	}
	if rule.TitlePattern == nil || *rule.TitlePattern != "*claude*" {
		t.Errorf("expected title_pattern '*claude*', got %v", rule.TitlePattern)
	}
	if rule.TagName != "Development" {
		t.Errorf("expected tag name 'Development', got %q", rule.TagName)
	}
}

func TestStore_ListAll_EvaluationOrder(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	titlePat := "*claude*"
	store.Create(1, "terminal", &titlePat, 10, false) // title-pattern, non-suggested
	store.Create(2, "slack", nil, 5, false)            // app-only, non-suggested
	store.Create(1, "firefox", nil, 3, true)           // app-only, suggested

	rules, err := store.ListAll()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// Order: title-pattern non-suggested, app-only non-suggested, suggested
	if rules[0].AppPattern != "terminal" {
		t.Errorf("first rule should be terminal (title-pattern), got %q", rules[0].AppPattern)
	}
	if rules[1].AppPattern != "slack" {
		t.Errorf("second rule should be slack (app-only), got %q", rules[1].AppPattern)
	}
	if rules[2].AppPattern != "firefox" {
		t.Errorf("third rule should be firefox (suggested), got %q", rules[2].AppPattern)
	}
}

func TestStore_IncrementHitCount(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	id, _ := store.Create(1, "terminal", nil, 5, false)

	for i := 0; i < 3; i++ {
		if err := store.IncrementHitCount(id); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	rule, _ := store.GetByID(id)
	if rule.HitCount != 3 {
		t.Errorf("expected hit_count 3, got %d", rule.HitCount)
	}
}

func TestStore_AcceptSuggested(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	id, _ := store.Create(1, "terminal", nil, 5, true)

	rule, _ := store.GetByID(id)
	if !rule.Suggested {
		t.Fatal("expected suggested=true")
	}

	if err := store.AcceptSuggested(id); err != nil {
		t.Fatalf("accept: %v", err)
	}

	rule, _ = store.GetByID(id)
	if rule.Suggested {
		t.Error("expected suggested=false after accept")
	}
}

func TestStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	id, _ := store.Create(1, "terminal", nil, 5, false)

	if err := store.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.GetByID(id)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestStore_Update(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	id, _ := store.Create(1, "terminal", nil, 5, false)

	newTitle := "*ssh*"
	if err := store.Update(id, "terminal", &newTitle, 15); err != nil {
		t.Fatalf("update: %v", err)
	}

	rule, _ := store.GetByID(id)
	if rule.Priority != 15 {
		t.Errorf("expected priority 15, got %d", rule.Priority)
	}
	if rule.TitlePattern == nil || *rule.TitlePattern != "*ssh*" {
		t.Errorf("expected title '*ssh*', got %v", rule.TitlePattern)
	}
}
