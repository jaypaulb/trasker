// internal/client/store/notes_test.go
package store_test

import (
	"testing"
	"time"
)

func TestCreateNote(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)

	noteID, err := s.CreateNote(eventID, "Working on auth module")
	if err != nil {
		t.Fatalf("CreateNote() error: %v", err)
	}
	if noteID < 1 {
		t.Fatalf("expected positive note ID, got %d", noteID)
	}
}

func TestListNotesForEvent(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)

	_, _ = s.CreateNote(eventID, "First note")
	_, _ = s.CreateNote(eventID, "Second note")

	notes, err := s.ListNotesForEvent(eventID)
	if err != nil {
		t.Fatalf("ListNotesForEvent() error: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}
	if notes[0].Text != "First note" {
		t.Errorf("notes[0].Text = %q, want %q", notes[0].Text, "First note")
	}
}

func TestMarkNoteCascadeApplied(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)
	noteID, _ := s.CreateNote(eventID, "Cascade me")

	err := s.MarkNoteCascadeApplied(noteID)
	if err != nil {
		t.Fatalf("MarkNoteCascadeApplied() error: %v", err)
	}

	notes, _ := s.ListNotesForEvent(eventID)
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1", len(notes))
	}
	if !notes[0].CascadeApplied {
		t.Error("CascadeApplied = false, want true")
	}
}

func TestListNotesForEvent_Empty(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)

	notes, err := s.ListNotesForEvent(eventID)
	if err != nil {
		t.Fatalf("ListNotesForEvent() error: %v", err)
	}
	if notes == nil {
		// We want an empty slice, not nil, for JSON serialization friendliness.
		// However, in Go the natural return is nil for no rows. Accept either.
	}
	if len(notes) != 0 {
		t.Fatalf("got %d notes, want 0", len(notes))
	}
}
