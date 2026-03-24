// internal/client/store/submissions_test.go
package store_test

import (
	"testing"
	"time"
)

func TestCreateSubmission(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	id, err := s.CreateSubmission(now)
	if err != nil {
		t.Fatalf("CreateSubmission() error: %v", err)
	}
	if id < 1 {
		t.Fatalf("expected positive ID, got %d", id)
	}
}

func TestLinkEventToSubmission(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	eventID, _ := s.InsertFocusEvent("Code", "main.go", now)
	subID, _ := s.CreateSubmission(now)

	err := s.LinkEventToSubmission(subID, eventID)
	if err != nil {
		t.Fatalf("LinkEventToSubmission() error: %v", err)
	}

	// Verify via IsEventSubmitted
	submitted, _ := s.IsEventSubmitted(eventID)
	if !submitted {
		t.Error("expected event to be submitted after linking")
	}
}

func TestUpdateSubmissionStatus(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	subID, _ := s.CreateSubmission(now)

	err := s.UpdateSubmissionStatus(subID, "submitted", "server-uuid-123")
	if err != nil {
		t.Fatalf("UpdateSubmissionStatus() error: %v", err)
	}

	sub, err := s.GetSubmission(subID)
	if err != nil {
		t.Fatalf("GetSubmission() error: %v", err)
	}
	if sub.Status != "submitted" {
		t.Errorf("Status = %q, want %q", sub.Status, "submitted")
	}
	if sub.ServerID == nil || *sub.ServerID != "server-uuid-123" {
		t.Errorf("ServerID = %v, want %q", sub.ServerID, "server-uuid-123")
	}
}

func TestListPendingSubmissions(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()

	// Create 2 pending and 1 confirmed
	sub1, _ := s.CreateSubmission(now)
	sub2, _ := s.CreateSubmission(now.Add(time.Minute))
	sub3, _ := s.CreateSubmission(now.Add(2 * time.Minute))
	_ = s.UpdateSubmissionStatus(sub3, "confirmed", "server-id")

	pending, err := s.ListPendingSubmissions()
	if err != nil {
		t.Fatalf("ListPendingSubmissions() error: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("got %d pending, want 2", len(pending))
	}
	_ = sub1
	_ = sub2
}

func TestGetSubmittedEventIDs(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	e1, _ := s.InsertFocusEvent("Code", "a.go", now)
	e2, _ := s.InsertFocusEvent("Code", "b.go", now.Add(time.Minute))
	e3, _ := s.InsertFocusEvent("Slack", "chat", now.Add(2*time.Minute))

	subID, _ := s.CreateSubmission(now)
	_ = s.LinkEventToSubmission(subID, e1)
	_ = s.LinkEventToSubmission(subID, e2)
	// e3 is NOT linked

	ids, err := s.GetSubmittedEventIDs()
	if err != nil {
		t.Fatalf("GetSubmittedEventIDs() error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d submitted IDs, want 2", len(ids))
	}
	_ = e3
}

func TestIncrementSubmissionRetry(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	subID, _ := s.CreateSubmission(now)

	err := s.IncrementSubmissionRetry(subID)
	if err != nil {
		t.Fatalf("IncrementSubmissionRetry() error: %v", err)
	}

	sub, _ := s.GetSubmission(subID)
	if sub.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", sub.RetryCount)
	}
	if sub.LastRetry == nil {
		t.Error("LastRetry is nil, want a timestamp")
	}
}
