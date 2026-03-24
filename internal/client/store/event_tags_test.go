// internal/client/store/event_tags_test.go
package store_test

import (
	"testing"
	"time"
)

func TestApplyTagToEvent(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)
	tagID, _ := s.CreateTag("Dev", "#0000FF")

	err := s.ApplyTagToEvent(eventID, tagID, "manual", nil)
	if err != nil {
		t.Fatalf("ApplyTagToEvent() error: %v", err)
	}

	tags, err := s.ListTagsForEvent(eventID)
	if err != nil {
		t.Fatalf("ListTagsForEvent() error: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("got %d tags, want 1", len(tags))
	}
	if tags[0].TagID != tagID {
		t.Errorf("TagID = %d, want %d", tags[0].TagID, tagID)
	}
	if tags[0].Source != "manual" {
		t.Errorf("Source = %q, want %q", tags[0].Source, "manual")
	}
}

func TestApplyTagToEvent_CascadeSource(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	anchorID, _ := s.InsertFocusEvent("Code", "anchor.go", now)
	targetID, _ := s.InsertFocusEvent("Code", "target.go", now.Add(5*time.Minute))
	tagID, _ := s.CreateTag("Dev", "#0000FF")

	cascadeFrom := anchorID
	err := s.ApplyTagToEvent(targetID, tagID, "cascade", &cascadeFrom)
	if err != nil {
		t.Fatalf("ApplyTagToEvent(cascade) error: %v", err)
	}

	tags, _ := s.ListTagsForEvent(targetID)
	if len(tags) != 1 {
		t.Fatalf("got %d tags, want 1", len(tags))
	}
	if tags[0].CascadeFrom == nil || *tags[0].CascadeFrom != anchorID {
		t.Errorf("CascadeFrom = %v, want %d", tags[0].CascadeFrom, anchorID)
	}
}

func TestApplyTagToEvent_DuplicateIgnored(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)
	tagID, _ := s.CreateTag("Dev", "#0000FF")

	_ = s.ApplyTagToEvent(eventID, tagID, "manual", nil)
	// Applying the same tag again should not error (upsert/ignore behavior)
	err := s.ApplyTagToEvent(eventID, tagID, "manual", nil)
	if err != nil {
		t.Fatalf("duplicate ApplyTagToEvent() error: %v", err)
	}

	tags, _ := s.ListTagsForEvent(eventID)
	if len(tags) != 1 {
		t.Fatalf("got %d tags after duplicate apply, want 1", len(tags))
	}
}

func TestListEventsForTag(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	e1, _ := s.InsertFocusEvent("Code", "a.go", now)
	e2, _ := s.InsertFocusEvent("Code", "b.go", now.Add(time.Minute))
	e3, _ := s.InsertFocusEvent("Slack", "chat", now.Add(2*time.Minute))
	tagID, _ := s.CreateTag("Dev", "#0000FF")

	_ = s.ApplyTagToEvent(e1, tagID, "manual", nil)
	_ = s.ApplyTagToEvent(e2, tagID, "auto_rule", nil)
	// e3 is NOT tagged

	eventIDs, err := s.ListEventIDsForTag(tagID)
	if err != nil {
		t.Fatalf("ListEventIDsForTag() error: %v", err)
	}
	if len(eventIDs) != 2 {
		t.Fatalf("got %d event IDs, want 2", len(eventIDs))
	}
	_ = e3 // keep the variable used
}

func TestIsEventSubmitted(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)

	// Not submitted yet
	submitted, err := s.IsEventSubmitted(eventID)
	if err != nil {
		t.Fatalf("IsEventSubmitted() error: %v", err)
	}
	if submitted {
		t.Error("expected not submitted, got true")
	}

	// Create a submission and link the event
	subID, _ := s.CreateSubmission(now)
	_ = s.LinkEventToSubmission(subID, eventID)

	submitted, err = s.IsEventSubmitted(eventID)
	if err != nil {
		t.Fatalf("IsEventSubmitted() error: %v", err)
	}
	if !submitted {
		t.Error("expected submitted, got false")
	}
}
