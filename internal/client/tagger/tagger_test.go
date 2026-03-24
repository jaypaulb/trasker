// internal/client/tagger/tagger_test.go
package tagger

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
)

func TestTagger_ProcessFocusEvent_AutoTags(t *testing.T) {
	db := setupTestDB(t)
	// Seed a rule
	db.Exec(`INSERT INTO tag_rules (tag_id, app_pattern, title_pattern, priority, suggested, hit_count, created_at)
	         VALUES (1, 'terminal', '*claude*', 10, 0, 0, '2026-01-01T00:00:00Z')`)

	var mu sync.Mutex
	applied := make(map[int64]int64) // eventID -> tagID

	applier := func(eventID, tagID, ruleID int64) error {
		mu.Lock()
		applied[eventID] = tagID
		mu.Unlock()
		return nil
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tg, err := NewTagger(db, applier, logger, 5)
	if err != nil {
		t.Fatalf("new tagger: %v", err)
	}

	tg.ProcessFocusEvent(context.Background(), FocusEvent{
		ID:          42,
		AppName:     "terminal",
		WindowTitle: "claude - trasker",
	})

	mu.Lock()
	tagID, ok := applied[42]
	mu.Unlock()

	if !ok {
		t.Fatal("expected event 42 to be tagged")
	}
	if tagID != 1 {
		t.Errorf("expected tagID 1, got %d", tagID)
	}
}

func TestTagger_ProcessFocusEvent_NoMatch(t *testing.T) {
	db := setupTestDB(t)

	applierCalled := false
	applier := func(eventID, tagID, ruleID int64) error {
		applierCalled = true
		return nil
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tg, err := NewTagger(db, applier, logger, 5)
	if err != nil {
		t.Fatalf("new tagger: %v", err)
	}

	tg.ProcessFocusEvent(context.Background(), FocusEvent{
		ID:          99,
		AppName:     "unknown",
		WindowTitle: "nothing",
	})

	if applierCalled {
		t.Error("applier should not have been called for non-matching event")
	}
}

func TestTagger_HitCountIncrements(t *testing.T) {
	db := setupTestDB(t)
	db.Exec(`INSERT INTO tag_rules (id, tag_id, app_pattern, priority, suggested, hit_count, created_at)
	         VALUES (100, 2, 'slack', 5, 0, 0, '2026-01-01T00:00:00Z')`)

	applier := func(eventID, tagID, ruleID int64) error { return nil }
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	tg, err := NewTagger(db, applier, logger, 5)
	if err != nil {
		t.Fatal(err)
	}

	tg.ProcessFocusEvent(context.Background(), FocusEvent{ID: 1, AppName: "slack", WindowTitle: "general"})
	tg.ProcessFocusEvent(context.Background(), FocusEvent{ID: 2, AppName: "slack", WindowTitle: "random"})

	rule, _ := tg.Store().GetByID(100)
	if rule.HitCount != 2 {
		t.Errorf("expected hit_count 2, got %d", rule.HitCount)
	}
}
