// internal/client/tagger/tagger.go
package tagger

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
)

// FocusEvent is the minimal focus event data needed by the tagger.
// This mirrors the relevant fields from the client store's focus event.
type FocusEvent struct {
	ID          int64
	AppName     string
	WindowTitle string
}

// TagApplier is called when the tagger auto-tags an event.
// Implementations should insert into event_tags with source="auto_rule".
type TagApplier func(eventID int64, tagID int64, ruleID int64) error

// Tagger orchestrates rule evaluation, hit counting, and learning.
type Tagger struct {
	engine  *Engine
	store   *Store
	learner *Learner
	applier TagApplier
	logger  *slog.Logger

	mu    sync.RWMutex
	rules []Rule // cached rules, refreshed on mutation
}

// NewTagger creates a fully wired tagger.
func NewTagger(db *sql.DB, applier TagApplier, logger *slog.Logger, learnerThreshold int) (*Tagger, error) {
	store := NewStore(db)
	engine := NewEngine()
	learner := NewLearner(db, store, learnerThreshold)

	tg := &Tagger{
		engine:  engine,
		store:   store,
		learner: learner,
		applier: applier,
		logger:  logger,
	}

	if err := tg.RefreshRules(); err != nil {
		return nil, err
	}

	return tg, nil
}

// Suggestions returns the channel for receiving suggested rule notifications.
func (tg *Tagger) Suggestions() <-chan Suggestion {
	return tg.learner.Suggestions
}

// Store returns the underlying rule store for direct CRUD access.
func (tg *Tagger) Store() *Store {
	return tg.store
}

// RefreshRules reloads rules from the database.
func (tg *Tagger) RefreshRules() error {
	rules, err := tg.store.ListAll()
	if err != nil {
		return err
	}
	tg.mu.Lock()
	tg.rules = rules
	tg.mu.Unlock()
	return nil
}

// ProcessFocusEvent evaluates rules against a new focus event and auto-tags if matched.
func (tg *Tagger) ProcessFocusEvent(ctx context.Context, event FocusEvent) {
	tg.mu.RLock()
	rules := tg.rules
	tg.mu.RUnlock()

	fc := FocusContext{
		AppName:     event.AppName,
		WindowTitle: event.WindowTitle,
	}

	result := tg.engine.Evaluate(fc, rules)
	if result == nil {
		return
	}

	tg.logger.Debug("auto-tag matched",
		"event_id", event.ID,
		"rule_id", result.Rule.ID,
		"tag", result.Rule.TagName,
	)

	if err := tg.applier(event.ID, result.TagID, result.Rule.ID); err != nil {
		tg.logger.Error("failed to apply auto-tag",
			"event_id", event.ID,
			"error", err,
		)
		return
	}

	if err := tg.store.IncrementHitCount(result.Rule.ID); err != nil {
		tg.logger.Warn("failed to increment hit_count",
			"rule_id", result.Rule.ID,
			"error", err,
		)
	}
}

// ObserveManualTag notifies the learner of a manual tagging event.
func (tg *Tagger) ObserveManualTag(appName, windowTitle string, tagID int64, tagName string) {
	tg.learner.ObserveManualTag(appName, windowTitle, tagID, tagName)
}
