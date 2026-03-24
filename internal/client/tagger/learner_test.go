// internal/client/tagger/learner_test.go
package tagger

import (
	"testing"
	"time"
)

func TestLearner_SuggestsAfterThreshold(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	learner := NewLearner(db, store, 3)

	// Observe the same pattern 3 times
	for i := 0; i < 3; i++ {
		learner.ObserveManualTag("terminal", "ssh - server-prod", 1, "Development")
	}

	// Should receive a suggestion
	select {
	case s := <-learner.Suggestions:
		if s.TagID != 1 {
			t.Errorf("expected TagID 1, got %d", s.TagID)
		}
		if s.AppPattern != "terminal" {
			t.Errorf("expected app 'terminal', got %q", s.AppPattern)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected suggestion, got none")
	}
}

func TestLearner_NoSuggestionBelowThreshold(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	learner := NewLearner(db, store, 5)

	// Observe pattern only 3 times (threshold is 5)
	for i := 0; i < 3; i++ {
		learner.ObserveManualTag("slack", "general - Slack", 2, "Communication")
	}

	select {
	case s := <-learner.Suggestions:
		t.Errorf("expected no suggestion, got %+v", s)
	case <-time.After(100 * time.Millisecond):
		// Expected: no suggestion
	}
}

func TestLearner_CreatesSuggestedRuleInStore(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	learner := NewLearner(db, store, 2)

	learner.ObserveManualTag("terminal", "claude - project", 1, "Development")
	learner.ObserveManualTag("terminal", "claude - other-project", 1, "Development")

	// Drain suggestion
	select {
	case <-learner.Suggestions:
	case <-time.After(100 * time.Millisecond):
	}

	rules, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}

	hasSuggested := false
	for _, r := range rules {
		if r.Suggested {
			hasSuggested = true
		}
	}
	if !hasSuggested {
		t.Error("expected a suggested rule in store")
	}
}

func TestExtractSignificantWords(t *testing.T) {
	words := extractSignificantWords("main.go - trasker - Visual Studio Code")
	if len(words) == 0 {
		t.Fatal("expected words")
	}
	// Should contain "main.go", "trasker", "visual", "studio", "code"
	found := false
	for _, w := range words {
		if w == "trasker" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'trasker' in words, got %v", words)
	}
}

func TestLearner_NoDuplicateSuggestions(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	learner := NewLearner(db, store, 2)

	// Hit threshold
	learner.ObserveManualTag("terminal", "claude work", 1, "Development")
	learner.ObserveManualTag("terminal", "claude session", 1, "Development")

	// Drain
	select {
	case <-learner.Suggestions:
	case <-time.After(100 * time.Millisecond):
	}

	// Continue past threshold — should not create duplicate
	learner.ObserveManualTag("terminal", "claude again", 1, "Development")

	select {
	case s := <-learner.Suggestions:
		t.Errorf("expected no duplicate suggestion, got %+v", s)
	case <-time.After(100 * time.Millisecond):
		// Expected: no duplicate
	}
}
