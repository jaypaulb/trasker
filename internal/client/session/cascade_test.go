// internal/client/session/cascade_test.go
package session_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jaypaulb/trasker/internal/client/session"
	"github.com/jaypaulb/trasker/internal/client/store"
)

func newCascadeTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.New() error: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// insertEvent is a test helper that creates a focus event with start and end times.
func insertEvent(t *testing.T, s *store.Store, app, title string, start time.Time, durationMin int) int64 {
	t.Helper()
	id, err := s.InsertFocusEvent(app, title, start)
	if err != nil {
		t.Fatalf("InsertFocusEvent() error: %v", err)
	}
	end := start.Add(time.Duration(durationMin) * time.Minute)
	if err := s.EndFocusEvent(id, end); err != nil {
		t.Fatalf("EndFocusEvent() error: %v", err)
	}
	return id
}

func TestCascade_BasicSpread(t *testing.T) {
	s := newCascadeTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	tagID, _ := s.CreateTag("Development", "#3B82F6")

	// Create events at regular intervals around the anchor:
	// 08:00, 08:20, 08:40, 09:00 (anchor), 09:20, 09:40, 10:00
	var events []int64
	for i := -3; i <= 3; i++ {
		offset := time.Duration(i*20) * time.Minute
		id := insertEvent(t, s, "Code", "file.go", base.Add(offset), 15)
		events = append(events, id)
	}

	anchorID := events[3] // 09:00

	cascader := session.NewCascader(s)
	tagged, err := cascader.Apply(anchorID, tagID)
	if err != nil {
		t.Fatalf("Cascade.Apply() error: %v", err)
	}

	// All 7 events are within ±60min of anchor, so step 1 tags them all.
	// The anchor itself should also be tagged.
	if len(tagged) < 7 {
		t.Errorf("got %d tagged events, want at least 7", len(tagged))
	}

	// Verify the anchor event has the tag
	tags, _ := s.ListTagsForEvent(anchorID)
	if len(tags) == 0 {
		t.Error("anchor event has no tags after cascade")
	}
}

func TestCascade_SkipsSubmittedEvents(t *testing.T) {
	s := newCascadeTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	tagID, _ := s.CreateTag("Dev", "#3B82F6")

	// Anchor at 09:00, neighbor at 09:10, submitted neighbor at 09:20
	anchorID := insertEvent(t, s, "Code", "anchor.go", base, 5)
	neighborID := insertEvent(t, s, "Code", "neighbor.go", base.Add(10*time.Minute), 5)
	submittedID := insertEvent(t, s, "Code", "submitted.go", base.Add(20*time.Minute), 5)

	// Mark the third event as submitted
	subID, _ := s.CreateSubmission(time.Now().UTC())
	_ = s.LinkEventToSubmission(subID, submittedID)

	cascader := session.NewCascader(s)
	tagged, err := cascader.Apply(anchorID, tagID)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	// Should have tagged anchor + neighbor, NOT submitted
	taggedSet := make(map[int64]bool)
	for _, id := range tagged {
		taggedSet[id] = true
	}

	if !taggedSet[anchorID] {
		t.Error("anchor was not tagged")
	}
	if !taggedSet[neighborID] {
		t.Error("neighbor was not tagged")
	}
	if taggedSet[submittedID] {
		t.Error("submitted event should NOT be tagged")
	}
}

func TestCascade_DecayStopsAtGap(t *testing.T) {
	s := newCascadeTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	tagID, _ := s.CreateTag("Dev", "#3B82F6")

	// Cluster 1: 09:00 (anchor), 09:10
	anchorID := insertEvent(t, s, "Code", "anchor.go", base, 5)
	nearID := insertEvent(t, s, "Code", "near.go", base.Add(10*time.Minute), 5)

	// Gap of 3 hours

	// Cluster 2: 12:00, 12:10 (far from any edge of cluster 1)
	farID1 := insertEvent(t, s, "Code", "far1.go", base.Add(3*time.Hour), 5)
	farID2 := insertEvent(t, s, "Code", "far2.go", base.Add(3*time.Hour+10*time.Minute), 5)

	cascader := session.NewCascader(s)
	tagged, err := cascader.Apply(anchorID, tagID)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	taggedSet := make(map[int64]bool)
	for _, id := range tagged {
		taggedSet[id] = true
	}

	if !taggedSet[anchorID] {
		t.Error("anchor was not tagged")
	}
	if !taggedSet[nearID] {
		t.Error("near event should be tagged")
	}
	if taggedSet[farID1] {
		t.Error("far event 1 should NOT be tagged (3hr gap)")
	}
	if taggedSet[farID2] {
		t.Error("far event 2 should NOT be tagged")
	}
}

func TestCascade_EmptyStore(t *testing.T) {
	s := newCascadeTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	tagID, _ := s.CreateTag("Dev", "#3B82F6")
	anchorID := insertEvent(t, s, "Code", "only.go", base, 5)

	cascader := session.NewCascader(s)
	tagged, err := cascader.Apply(anchorID, tagID)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	// Should tag just the anchor
	if len(tagged) != 1 {
		t.Errorf("got %d tagged, want 1 (just anchor)", len(tagged))
	}
	if tagged[0] != anchorID {
		t.Errorf("tagged[0] = %d, want anchor %d", tagged[0], anchorID)
	}
}

func TestCascade_TagSourceIsCascade(t *testing.T) {
	s := newCascadeTestStore(t)

	base := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	tagID, _ := s.CreateTag("Dev", "#3B82F6")

	anchorID := insertEvent(t, s, "Code", "anchor.go", base, 5)
	neighborID := insertEvent(t, s, "Code", "neighbor.go", base.Add(10*time.Minute), 5)

	cascader := session.NewCascader(s)
	_, _ = cascader.Apply(anchorID, tagID)

	// Anchor should have source "manual" (set by cascade as the origin)
	anchorTags, _ := s.ListTagsForEvent(anchorID)
	if len(anchorTags) < 1 {
		t.Fatal("anchor has no tags")
	}
	if anchorTags[0].Source != "manual" {
		t.Errorf("anchor tag source = %q, want %q", anchorTags[0].Source, "manual")
	}

	// Neighbor should have source "cascade" with cascade_from pointing to anchor
	neighborTags, _ := s.ListTagsForEvent(neighborID)
	if len(neighborTags) < 1 {
		t.Fatal("neighbor has no tags")
	}
	if neighborTags[0].Source != "cascade" {
		t.Errorf("neighbor tag source = %q, want %q", neighborTags[0].Source, "cascade")
	}
	if neighborTags[0].CascadeFrom == nil || *neighborTags[0].CascadeFrom != anchorID {
		t.Errorf("neighbor CascadeFrom = %v, want %d", neighborTags[0].CascadeFrom, anchorID)
	}
}
