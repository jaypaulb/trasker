// internal/client/store/pomodoro_test.go
package store

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupPomodoroDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE tags (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			color TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE pomodoro_sessions (
			id INTEGER PRIMARY KEY,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			work_mins INTEGER NOT NULL DEFAULT 25,
			break_mins INTEGER NOT NULL DEFAULT 5,
			status TEXT NOT NULL,
			tag_id INTEGER REFERENCES tags(id),
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Dev', '#00ff00', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func TestPomodoroStore_CreateAndGet(t *testing.T) {
	db := setupPomodoroDB(t)
	s := NewPomodoroStore(db)

	tagID := int64(1)
	id, err := s.Create(25, 5, "work", &tagID)
	if err != nil {
		t.Fatal(err)
	}

	ps, err := s.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if ps.WorkMins != 25 {
		t.Errorf("expected 25, got %d", ps.WorkMins)
	}
	if ps.Status != "work" {
		t.Errorf("expected 'work', got %q", ps.Status)
	}
	if ps.TagID == nil || *ps.TagID != 1 {
		t.Errorf("expected tagID 1, got %v", ps.TagID)
	}
	if ps.EndedAt != nil {
		t.Error("expected nil EndedAt")
	}
}

func TestPomodoroStore_End(t *testing.T) {
	db := setupPomodoroDB(t)
	s := NewPomodoroStore(db)

	id, _ := s.Create(25, 5, "work", nil)
	if err := s.End(id, "done"); err != nil {
		t.Fatal(err)
	}

	ps, _ := s.GetByID(id)
	if ps.Status != "done" {
		t.Errorf("expected 'done', got %q", ps.Status)
	}
	if ps.EndedAt == nil {
		t.Error("expected EndedAt to be set")
	}
}

func TestPomodoroStore_UpdateStatus(t *testing.T) {
	db := setupPomodoroDB(t)
	s := NewPomodoroStore(db)

	id, _ := s.Create(25, 5, "work", nil)
	if err := s.UpdateStatus(id, "break"); err != nil {
		t.Fatal(err)
	}

	ps, _ := s.GetByID(id)
	if ps.Status != "break" {
		t.Errorf("expected 'break', got %q", ps.Status)
	}
}

func TestPomodoroStore_ListRecent(t *testing.T) {
	db := setupPomodoroDB(t)
	s := NewPomodoroStore(db)

	s.Create(25, 5, "done", nil)
	s.Create(25, 5, "done", nil)
	s.Create(25, 5, "work", nil)

	sessions, err := s.ListRecent(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestPomodoroStore_ListByDate(t *testing.T) {
	db := setupPomodoroDB(t)
	s := NewPomodoroStore(db)

	s.Create(25, 5, "done", nil)

	sessions, err := s.ListByDate(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session today, got %d", len(sessions))
	}
}
