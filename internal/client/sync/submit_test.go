// internal/client/sync/submit_test.go
package sync

import (
	"testing"
	"time"
)

func TestAggregate_GroupsByTag(t *testing.T) {
	svc := &SubmitService{}

	events := []RawEvent{
		{ID: 1, AppName: "code", StartedAt: mustTime("09:00"), EndedAt: mustTime("10:00"),
			DurationS: 3600, Tag: "Dev"},
		{ID: 2, AppName: "terminal", StartedAt: mustTime("10:00"), EndedAt: mustTime("10:30"),
			DurationS: 1800, Tag: "Dev"},
		{ID: 3, AppName: "slack", StartedAt: mustTime("10:30"), EndedAt: mustTime("11:00"),
			DurationS: 1800, Tag: "Comms"},
	}

	result := svc.Aggregate(events)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}

	// Find Dev group
	var dev, comms *AggregatedEntry
	for i := range result {
		switch result[i].Tag {
		case "Dev":
			dev = &result[i]
		case "Comms":
			comms = &result[i]
		}
	}

	if dev == nil || comms == nil {
		t.Fatal("expected both Dev and Comms groups")
	}

	if dev.DurationS != 5400 {
		t.Errorf("Dev duration: expected 5400, got %d", dev.DurationS)
	}
	if len(dev.EventIDs) != 2 {
		t.Errorf("Dev events: expected 2, got %d", len(dev.EventIDs))
	}
	if comms.DurationS != 1800 {
		t.Errorf("Comms duration: expected 1800, got %d", comms.DurationS)
	}
}

func TestAggregate_AppSummary(t *testing.T) {
	svc := &SubmitService{}

	events := []RawEvent{
		{ID: 1, AppName: "VS Code", StartedAt: mustTime("09:00"), EndedAt: mustTime("10:00"),
			DurationS: 3600, Tag: "Dev"},
		{ID: 2, AppName: "Terminal", StartedAt: mustTime("10:00"), EndedAt: mustTime("10:30"),
			DurationS: 900, Tag: "Dev"},
	}

	result := svc.Aggregate(events)
	if len(result) != 1 {
		t.Fatal("expected 1 group")
	}

	summary := result[0].AppSummary
	if summary == "" {
		t.Error("expected non-empty app summary")
	}
	// VS Code should be 80%, Terminal 20%
	t.Logf("app_summary: %s", summary)
}

func TestAggregate_NotesConcatenated(t *testing.T) {
	svc := &SubmitService{}
	noteTime1 := mustTime("09:15")
	noteTime2 := mustTime("10:30")

	events := []RawEvent{
		{ID: 1, AppName: "code", StartedAt: mustTime("09:00"), EndedAt: mustTime("10:00"),
			DurationS: 3600, Tag: "Dev", NoteText: "Working on auth", NoteTime: &noteTime1},
		{ID: 2, AppName: "code", StartedAt: mustTime("10:00"), EndedAt: mustTime("11:00"),
			DurationS: 3600, Tag: "Dev", NoteText: "API tests", NoteTime: &noteTime2},
	}

	result := svc.Aggregate(events)
	if len(result) != 1 {
		t.Fatal("expected 1 group")
	}

	notes := result[0].Notes
	if notes == "" {
		t.Fatal("expected notes")
	}
	// Should contain timestamps
	if !containsSubstring(notes, "[09:15]") || !containsSubstring(notes, "[10:30]") {
		t.Errorf("expected timestamps in notes, got: %s", notes)
	}
}

func TestCreateSubmission(t *testing.T) {
	db := setupQueueDB(t)
	svc := NewSubmitService(db)

	id, err := svc.CreateSubmission([]int64{1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Verify submission record
	var status string
	db.QueryRow(`SELECT status FROM submissions WHERE id = ?`, id).Scan(&status)
	if status != "pending" {
		t.Errorf("expected 'pending', got %q", status)
	}

	// Verify link
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM submission_events WHERE submission_id = ?`, id).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 linked event, got %d", count)
	}
}

func mustTime(hhmm string) time.Time {
	t, _ := time.Parse("15:04", hhmm)
	return time.Date(2026, 3, 23, t.Hour(), t.Minute(), 0, 0, time.UTC)
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
