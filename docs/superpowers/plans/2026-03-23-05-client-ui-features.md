# Client UI & Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Build client-side features — tagger, pomodoro, system tray, notifications, local web dashboard, server sync, first-run, autostart

**Architecture:** Go daemon serves Svelte SPA on localhost. System tray via getlantern/systray. Notifications via OS-native APIs. Tagger evaluates rules against focus events. Sync client handles submission queue with retry.

**Tech Stack:** Go 1.22+, SvelteKit (client-ui), systray library, go:embed for SPA assets

**Depends on:** Plan 01 (shared foundation — models, apikey, version), Plan 03 (client core — store, tracker, presence, session engine)

**Assumptions from Plan 01:** `internal/shared/models/` exists with API request/response types. `go.mod` initialized.
**Assumptions from Plan 03:** `internal/client/store/` exists with SQLite setup, `focus_events`/`tags`/`event_tags`/`notes`/`config` tables. `internal/client/tracker/` provides focus events via channel. `internal/client/presence/` provides presence state. `internal/client/session/` provides session engine with note cascade.

---

## Task 1: Tagger — Rule Engine

**Files:**
- Create: `internal/client/tagger/engine.go`
- Create: `internal/client/tagger/engine_test.go`

### Steps

- [ ] **1.1** Create directory and engine file with types

```bash
mkdir -p internal/client/tagger
```

```go
// internal/client/tagger/engine.go
package tagger

import (
	"path/filepath"
	"strings"
)

// Rule represents an auto-tag rule evaluated against focus events.
type Rule struct {
	ID           int64
	TagID        int64
	TagName      string
	AppPattern   string  // glob pattern for app_name
	TitlePattern *string // glob pattern for window_title (nil = app-only rule)
	Priority     int
	Suggested    bool
	HitCount     int64
}

// FocusContext contains the fields from a focus event needed for rule evaluation.
type FocusContext struct {
	AppName     string
	WindowTitle string
}

// MatchResult is the outcome of evaluating rules against a focus context.
type MatchResult struct {
	Rule  Rule
	TagID int64
}

// Engine evaluates tag rules against focus events.
type Engine struct{}

// NewEngine creates a new tagger engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Evaluate checks the given focus context against rules in priority order.
// Rules are expected to be pre-sorted: title-pattern first, then app-only,
// then suggested. Within each group, higher Priority values win.
// Returns nil if no rule matches.
func (e *Engine) Evaluate(ctx FocusContext, rules []Rule) *MatchResult {
	// Partition rules into tiers for evaluation order:
	// 1. Title-pattern rules (non-suggested)
	// 2. App-only rules (non-suggested)
	// 3. Suggested rules (title-pattern first, then app-only)
	var titleRules, appRules, suggestedTitle, suggestedApp []Rule
	for _, r := range rules {
		if r.Suggested {
			if r.TitlePattern != nil {
				suggestedTitle = append(suggestedTitle, r)
			} else {
				suggestedApp = append(suggestedApp, r)
			}
		} else if r.TitlePattern != nil {
			titleRules = append(titleRules, r)
		} else {
			appRules = append(appRules, r)
		}
	}

	// Evaluate tiers in order
	for _, tier := range [][]Rule{titleRules, appRules, suggestedTitle, suggestedApp} {
		if result := e.evaluateTier(ctx, tier); result != nil {
			return result
		}
	}
	return nil
}

// evaluateTier checks a single tier of rules. Returns the first match
// (rules within a tier are assumed sorted by descending Priority).
func (e *Engine) evaluateTier(ctx FocusContext, rules []Rule) *MatchResult {
	for _, r := range rules {
		if e.matchesRule(ctx, r) {
			return &MatchResult{Rule: r, TagID: r.TagID}
		}
	}
	return nil
}

// matchesRule checks if a focus context matches a single rule.
func (e *Engine) matchesRule(ctx FocusContext, r Rule) bool {
	appLower := strings.ToLower(ctx.AppName)
	patLower := strings.ToLower(r.AppPattern)

	matched, err := filepath.Match(patLower, appLower)
	if err != nil || !matched {
		return false
	}

	if r.TitlePattern != nil {
		titleLower := strings.ToLower(ctx.WindowTitle)
		titlePatLower := strings.ToLower(*r.TitlePattern)
		titleMatched, err := filepath.Match(titlePatLower, titleLower)
		if err != nil || !titleMatched {
			return false
		}
	}

	return true
}
```

- [ ] **1.2** Write tests for the engine

```go
// internal/client/tagger/engine_test.go
package tagger

import (
	"testing"
)

func strPtr(s string) *string { return &s }

func TestEvaluate_TitlePatternWins(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "terminal", WindowTitle: "claude - project"}

	rules := []Rule{
		{ID: 1, TagID: 10, TagName: "Dev", AppPattern: "terminal", TitlePattern: strPtr("*claude*"), Priority: 10},
		{ID: 2, TagID: 20, TagName: "Terminal", AppPattern: "terminal", Priority: 5},
	}

	result := engine.Evaluate(ctx, rules)
	if result == nil {
		t.Fatal("expected a match, got nil")
	}
	if result.TagID != 10 {
		t.Errorf("expected TagID 10, got %d", result.TagID)
	}
}

func TestEvaluate_AppOnlyFallback(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "slack", WindowTitle: "general - Slack"}

	rules := []Rule{
		{ID: 1, TagID: 10, TagName: "Dev", AppPattern: "terminal", TitlePattern: strPtr("*claude*"), Priority: 10},
		{ID: 2, TagID: 20, TagName: "Comms", AppPattern: "slack", Priority: 5},
	}

	result := engine.Evaluate(ctx, rules)
	if result == nil {
		t.Fatal("expected a match, got nil")
	}
	if result.TagID != 20 {
		t.Errorf("expected TagID 20, got %d", result.TagID)
	}
}

func TestEvaluate_SuggestedLowerPriority(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "firefox", WindowTitle: "GitHub - Mozilla Firefox"}

	rules := []Rule{
		{ID: 1, TagID: 10, TagName: "Browsing", AppPattern: "firefox", Suggested: true, Priority: 5},
		{ID: 2, TagID: 20, TagName: "Web", AppPattern: "firefox", Priority: 3},
	}

	// Non-suggested app-only rule should beat suggested, even with lower priority number
	result := engine.Evaluate(ctx, rules)
	if result == nil {
		t.Fatal("expected a match, got nil")
	}
	if result.TagID != 20 {
		t.Errorf("expected TagID 20 (non-suggested), got %d", result.TagID)
	}
}

func TestEvaluate_NoMatch(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "unknown-app", WindowTitle: "something"}

	rules := []Rule{
		{ID: 1, TagID: 10, TagName: "Dev", AppPattern: "terminal", Priority: 10},
	}

	result := engine.Evaluate(ctx, rules)
	if result != nil {
		t.Errorf("expected nil, got match with TagID %d", result.TagID)
	}
}

func TestEvaluate_CaseInsensitive(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "Firefox", WindowTitle: "GitHub - Mozilla Firefox"}

	rules := []Rule{
		{ID: 1, TagID: 10, TagName: "Web", AppPattern: "firefox", Priority: 5},
	}

	result := engine.Evaluate(ctx, rules)
	if result == nil {
		t.Fatal("expected a match, got nil")
	}
}

func TestEvaluate_EmptyRules(t *testing.T) {
	engine := NewEngine()
	ctx := FocusContext{AppName: "anything", WindowTitle: "whatever"}

	result := engine.Evaluate(ctx, nil)
	if result != nil {
		t.Error("expected nil for empty rules")
	}
}

func TestEvaluate_GlobPatterns(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name    string
		ctx     FocusContext
		rule    Rule
		matches bool
	}{
		{
			name:    "wildcard in title",
			ctx:     FocusContext{AppName: "code", WindowTitle: "main.go - trasker - Visual Studio Code"},
			rule:    Rule{ID: 1, TagID: 1, AppPattern: "code", TitlePattern: strPtr("*trasker*")},
			matches: true,
		},
		{
			name:    "question mark glob",
			ctx:     FocusContext{AppName: "code", WindowTitle: "a"},
			rule:    Rule{ID: 2, TagID: 2, AppPattern: "code", TitlePattern: strPtr("?")},
			matches: true,
		},
		{
			name:    "no match with different app",
			ctx:     FocusContext{AppName: "vim", WindowTitle: "main.go"},
			rule:    Rule{ID: 3, TagID: 3, AppPattern: "code", TitlePattern: strPtr("*")},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Evaluate(tt.ctx, []Rule{tt.rule})
			if tt.matches && result == nil {
				t.Error("expected match, got nil")
			}
			if !tt.matches && result != nil {
				t.Error("expected no match, got match")
			}
		})
	}
}
```

- [ ] **1.3** Run tests

```bash
cd /path/to/trasker && go test ./internal/client/tagger/ -v -run TestEvaluate
# Expected: all tests PASS
```

- [ ] **1.4** Commit

```bash
git add internal/client/tagger/engine.go internal/client/tagger/engine_test.go
git commit -m "feat(tagger): add rule evaluation engine with glob matching and priority tiers"
```

---

## Task 2: Tagger — Rule Store

**Files:**
- Create: `internal/client/tagger/store.go`
- Create: `internal/client/tagger/store_test.go`

### Steps

- [ ] **2.1** Create the rule store with CRUD and hit_count increment

```go
// internal/client/tagger/store.go
package tagger

import (
	"database/sql"
	"fmt"
	"time"
)

// Store handles persistence for tag rules in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new rule store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new tag rule. Returns the new rule ID.
func (s *Store) Create(tagID int64, appPattern string, titlePattern *string, priority int, suggested bool) (int64, error) {
	suggestedInt := 0
	if suggested {
		suggestedInt = 1
	}
	result, err := s.db.Exec(
		`INSERT INTO tag_rules (tag_id, app_pattern, title_pattern, priority, suggested, hit_count, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		tagID, appPattern, titlePattern, priority, suggestedInt, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("tagger store: create rule: %w", err)
	}
	return result.LastInsertId()
}

// GetByID fetches a single rule.
func (s *Store) GetByID(id int64) (*Rule, error) {
	row := s.db.QueryRow(
		`SELECT r.id, r.tag_id, t.name, r.app_pattern, r.title_pattern,
		        r.priority, r.suggested, r.hit_count
		 FROM tag_rules r
		 JOIN tags t ON t.id = r.tag_id
		 WHERE r.id = ?`, id,
	)
	return scanRule(row)
}

// ListAll returns all rules ordered for evaluation:
// non-suggested title-pattern first (desc priority), then non-suggested app-only,
// then suggested title-pattern, then suggested app-only.
func (s *Store) ListAll() ([]Rule, error) {
	rows, err := s.db.Query(
		`SELECT r.id, r.tag_id, t.name, r.app_pattern, r.title_pattern,
		        r.priority, r.suggested, r.hit_count
		 FROM tag_rules r
		 JOIN tags t ON t.id = r.tag_id
		 ORDER BY r.suggested ASC,
		          CASE WHEN r.title_pattern IS NOT NULL THEN 0 ELSE 1 END ASC,
		          r.priority DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("tagger store: list all: %w", err)
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		r, err := scanRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Update modifies a rule's patterns and priority.
func (s *Store) Update(id int64, appPattern string, titlePattern *string, priority int) error {
	result, err := s.db.Exec(
		`UPDATE tag_rules SET app_pattern = ?, title_pattern = ?, priority = ? WHERE id = ?`,
		appPattern, titlePattern, priority, id,
	)
	if err != nil {
		return fmt.Errorf("tagger store: update rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found", id)
	}
	return nil
}

// Delete removes a rule.
func (s *Store) Delete(id int64) error {
	result, err := s.db.Exec(`DELETE FROM tag_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("tagger store: delete rule: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found", id)
	}
	return nil
}

// IncrementHitCount bumps the hit_count for a rule.
func (s *Store) IncrementHitCount(id int64) error {
	_, err := s.db.Exec(`UPDATE tag_rules SET hit_count = hit_count + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("tagger store: increment hit_count: %w", err)
	}
	return nil
}

// AcceptSuggested promotes a suggested rule to a confirmed rule.
func (s *Store) AcceptSuggested(id int64) error {
	result, err := s.db.Exec(`UPDATE tag_rules SET suggested = 0 WHERE id = ? AND suggested = 1`, id)
	if err != nil {
		return fmt.Errorf("tagger store: accept suggested: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tagger store: rule %d not found or not suggested", id)
	}
	return nil
}

// scanner interface to support both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanRuleFromScanner(s scanner) (Rule, error) {
	var r Rule
	var titlePattern sql.NullString
	var suggested int

	err := s.Scan(&r.ID, &r.TagID, &r.TagName, &r.AppPattern, &titlePattern,
		&r.Priority, &suggested, &r.HitCount)
	if err != nil {
		return r, fmt.Errorf("tagger store: scan rule: %w", err)
	}
	if titlePattern.Valid {
		r.TitlePattern = &titlePattern.String
	}
	r.Suggested = suggested == 1
	return r, nil
}

func scanRule(row *sql.Row) (*Rule, error) {
	r, err := scanRuleFromScanner(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func scanRuleRow(rows *sql.Rows) (Rule, error) {
	return scanRuleFromScanner(rows)
}
```

- [ ] **2.2** Write tests using an in-memory SQLite database

```go
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
	store.Create(1, "terminal", &titlePat, 10, false)  // title-pattern, non-suggested
	store.Create(2, "slack", nil, 5, false)              // app-only, non-suggested
	store.Create(1, "firefox", nil, 3, true)             // app-only, suggested

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
```

- [ ] **2.3** Run tests

```bash
go test ./internal/client/tagger/ -v -run TestStore
# Expected: all tests PASS
```

- [ ] **2.4** Commit

```bash
git add internal/client/tagger/store.go internal/client/tagger/store_test.go
git commit -m "feat(tagger): add rule store with CRUD, hit_count, and suggested promotion"
```

---

## Task 3: Tagger — Learning Engine

**Files:**
- Create: `internal/client/tagger/learner.go`
- Create: `internal/client/tagger/learner_test.go`

### Steps

- [ ] **3.1** Create the learning engine that observes manual tagging patterns

```go
// internal/client/tagger/learner.go
package tagger

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

// Suggestion represents a learned pattern the system wants to propose as an auto-tag rule.
type Suggestion struct {
	TagID        int64
	TagName      string
	AppPattern   string
	TitlePattern *string // nil if app-only suggestion
	ObservedN    int     // number of times this pattern was observed
}

// PatternKey uniquely identifies a manual tagging pattern.
type PatternKey struct {
	AppName        string
	TitleSubstring string // empty string = app-only pattern
	TagID          int64
}

// Learner observes manual tagging events and surfaces suggested rules
// when a pattern occurs at least Threshold times.
type Learner struct {
	mu          sync.Mutex
	counts      map[PatternKey]int
	threshold   int
	store       *Store
	db          *sql.DB
	Suggestions chan Suggestion
}

// NewLearner creates a new learning engine.
// threshold is the number of observations before a suggestion is surfaced.
func NewLearner(db *sql.DB, store *Store, threshold int) *Learner {
	return &Learner{
		counts:      make(map[PatternKey]int),
		threshold:   threshold,
		store:       store,
		db:          db,
		Suggestions: make(chan Suggestion, 16),
	}
}

// ObserveManualTag records a manual tagging event and checks if a suggestion
// should be created. Call this when a user manually tags a focus event.
func (l *Learner) ObserveManualTag(appName string, windowTitle string, tagID int64, tagName string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Track app-only pattern
	appKey := PatternKey{
		AppName: strings.ToLower(appName),
		TagID:   tagID,
	}
	l.counts[appKey]++

	// Track app + title-word patterns
	// Extract significant words (>3 chars) from the title
	words := extractSignificantWords(windowTitle)
	for _, word := range words {
		titleKey := PatternKey{
			AppName:        strings.ToLower(appName),
			TitleSubstring: word,
			TagID:          tagID,
		}
		l.counts[titleKey]++

		if l.counts[titleKey] == l.threshold {
			l.proposeSuggestion(titleKey, tagName)
		}
	}

	// Check app-only pattern (only if no title patterns hit threshold)
	if l.counts[appKey] == l.threshold {
		l.proposeSuggestion(appKey, tagName)
	}
}

// proposeSuggestion creates a suggested rule in the store and notifies via channel.
func (l *Learner) proposeSuggestion(key PatternKey, tagName string) {
	// Check if a similar rule already exists
	existing, err := l.store.ListAll()
	if err != nil {
		return
	}
	for _, r := range existing {
		if strings.EqualFold(r.AppPattern, key.AppName) && r.TagID == key.TagID {
			if key.TitleSubstring == "" && r.TitlePattern == nil {
				return // duplicate app-only rule
			}
			if r.TitlePattern != nil && strings.Contains(
				strings.ToLower(*r.TitlePattern), key.TitleSubstring) {
				return // similar title pattern exists
			}
		}
	}

	var titlePat *string
	if key.TitleSubstring != "" {
		p := "*" + key.TitleSubstring + "*"
		titlePat = &p
	}

	_, err = l.store.Create(key.TagID, key.AppName, titlePat, 0, true)
	if err != nil {
		return
	}

	suggestion := Suggestion{
		TagID:        key.TagID,
		TagName:      tagName,
		AppPattern:   key.AppName,
		TitlePattern: titlePat,
		ObservedN:    l.threshold,
	}

	// Non-blocking send to suggestions channel
	select {
	case l.Suggestions <- suggestion:
	default:
	}
}

// extractSignificantWords pulls words >3 chars from a title, lowercased.
func extractSignificantWords(title string) []string {
	// Split on common separators
	replacer := strings.NewReplacer(
		"-", " ", "—", " ", "|", " ", "·", " ",
		"[", " ", "]", " ", "(", " ", ")", " ",
		"/", " ", "\\", " ", ":", " ",
	)
	cleaned := replacer.Replace(title)
	parts := strings.Fields(cleaned)

	var words []string
	seen := make(map[string]bool)
	for _, p := range parts {
		w := strings.ToLower(strings.TrimSpace(p))
		if len(w) > 3 && !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	return words
}
```

- [ ] **3.2** Write tests

```go
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
```

- [ ] **3.3** Run tests

```bash
go test ./internal/client/tagger/ -v -run TestLearner
# Expected: all tests PASS
```

- [ ] **3.4** Commit

```bash
git add internal/client/tagger/learner.go internal/client/tagger/learner_test.go
git commit -m "feat(tagger): add learning engine that surfaces auto-tag suggestions from manual patterns"
```

---

## Task 4: Tagger — Integration

**Files:**
- Create: `internal/client/tagger/tagger.go`
- Create: `internal/client/tagger/tagger_test.go`

### Steps

- [ ] **4.1** Create the top-level Tagger that wires engine + store + learner and integrates with the session engine

```go
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
```

- [ ] **4.2** Write integration test

```go
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
```

- [ ] **4.3** Run tests

```bash
go test ./internal/client/tagger/ -v -run TestTagger
# Expected: all tests PASS
```

- [ ] **4.4** Commit

```bash
git add internal/client/tagger/tagger.go internal/client/tagger/tagger_test.go
git commit -m "feat(tagger): add top-level Tagger wiring engine, store, and learner"
```

---

## Task 5: Pomodoro Timer

**Files:**
- Create: `internal/client/pomodoro/timer.go`
- Create: `internal/client/pomodoro/timer_test.go`

### Steps

- [ ] **5.1** Create the pomodoro state machine

```go
// internal/client/pomodoro/timer.go
package pomodoro

import (
	"fmt"
	"sync"
	"time"
)

// State represents the pomodoro timer state.
type State string

const (
	StateIdle   State = "idle"
	StateWork   State = "work"
	StateBreak  State = "break"
	StateDone   State = "done"
	StatePaused State = "paused"
)

// Config holds pomodoro timer settings.
type Config struct {
	WorkMins  int
	BreakMins int
}

// DefaultConfig returns 25/5 defaults.
func DefaultConfig() Config {
	return Config{WorkMins: 25, BreakMins: 5}
}

// StateChange is emitted when the timer transitions.
type StateChange struct {
	From      State
	To        State
	Remaining time.Duration
	TagID     *int64 // optional tag association
}

// Timer implements a pomodoro state machine.
type Timer struct {
	mu        sync.Mutex
	state     State
	config    Config
	tagID     *int64
	startedAt time.Time
	endAt     time.Time
	changes   chan StateChange
	stopCh    chan struct{}
	ticker    *time.Ticker
}

// NewTimer creates a new pomodoro timer. The changes channel receives state transitions.
func NewTimer(config Config) *Timer {
	return &Timer{
		state:   StateIdle,
		config:  config,
		changes: make(chan StateChange, 16),
	}
}

// Changes returns the channel for state transition notifications.
func (t *Timer) Changes() <-chan StateChange {
	return t.changes
}

// State returns the current timer state.
func (t *Timer) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// Remaining returns the time left in the current phase.
func (t *Timer) Remaining() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == StateIdle || t.state == StateDone {
		return 0
	}
	rem := time.Until(t.endAt)
	if rem < 0 {
		return 0
	}
	return rem
}

// Start begins a work phase. Optional tagID associates the session with an activity tag.
func (t *Timer) Start(tagID *int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateIdle && t.state != StateDone {
		return fmt.Errorf("pomodoro: cannot start from state %s", t.state)
	}

	t.tagID = tagID
	t.transitionTo(StateWork)
	return nil
}

// Cancel stops the timer and returns to idle.
func (t *Timer) Cancel() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopCh != nil {
		close(t.stopCh)
		t.stopCh = nil
	}
	if t.ticker != nil {
		t.ticker.Stop()
		t.ticker = nil
	}

	old := t.state
	t.state = StateIdle
	t.emitChange(old, StateIdle)
}

// transitionTo changes state and starts the appropriate countdown.
// Must be called with t.mu held.
func (t *Timer) transitionTo(newState State) {
	old := t.state

	// Stop previous ticker
	if t.stopCh != nil {
		close(t.stopCh)
	}
	if t.ticker != nil {
		t.ticker.Stop()
	}

	t.state = newState
	t.startedAt = time.Now()

	var duration time.Duration
	switch newState {
	case StateWork:
		duration = time.Duration(t.config.WorkMins) * time.Minute
	case StateBreak:
		duration = time.Duration(t.config.BreakMins) * time.Minute
	default:
		t.emitChange(old, newState)
		return
	}

	t.endAt = t.startedAt.Add(duration)
	t.emitChange(old, newState)

	stopCh := make(chan struct{})
	t.stopCh = stopCh
	t.ticker = time.NewTicker(1 * time.Second)

	go t.runPhase(newState, stopCh)
}

// runPhase waits for the phase to end, then transitions to the next state.
func (t *Timer) runPhase(phase State, stopCh chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		case <-time.After(time.Until(t.endAt)):
			t.mu.Lock()
			// Verify we're still in the expected phase
			if t.state != phase {
				t.mu.Unlock()
				return
			}
			switch phase {
			case StateWork:
				t.transitionTo(StateBreak)
			case StateBreak:
				old := t.state
				t.state = StateDone
				if t.ticker != nil {
					t.ticker.Stop()
					t.ticker = nil
				}
				t.emitChange(old, StateDone)
			}
			t.mu.Unlock()
			return
		}
	}
}

// emitChange sends a state change notification (non-blocking).
func (t *Timer) emitChange(from, to State) {
	sc := StateChange{
		From:      from,
		To:        to,
		Remaining: time.Until(t.endAt),
		TagID:     t.tagID,
	}
	select {
	case t.changes <- sc:
	default:
	}
}
```

- [ ] **5.2** Write tests

```go
// internal/client/pomodoro/timer_test.go
package pomodoro

import (
	"testing"
	"time"
)

func TestTimer_StartsInIdle(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	if timer.State() != StateIdle {
		t.Errorf("expected idle, got %s", timer.State())
	}
}

func TestTimer_StartTransitionsToWork(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	if err := timer.Start(nil); err != nil {
		t.Fatalf("start: %v", err)
	}

	if timer.State() != StateWork {
		t.Errorf("expected work, got %s", timer.State())
	}

	// Should receive a state change
	select {
	case sc := <-timer.Changes():
		if sc.From != StateIdle || sc.To != StateWork {
			t.Errorf("expected idle->work, got %s->%s", sc.From, sc.To)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected state change")
	}

	timer.Cancel()
}

func TestTimer_CannotStartWhenRunning(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)

	err := timer.Start(nil)
	if err == nil {
		t.Error("expected error starting while running")
	}

	timer.Cancel()
}

func TestTimer_CancelReturnsToIdle(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)
	timer.Cancel()

	if timer.State() != StateIdle {
		t.Errorf("expected idle after cancel, got %s", timer.State())
	}
}

func TestTimer_ShortWorkTransitionsToBreak(t *testing.T) {
	// Use 1-second work/break for fast test
	config := Config{WorkMins: 0, BreakMins: 0}
	timer := &Timer{
		state:   StateIdle,
		config:  config,
		changes: make(chan StateChange, 16),
	}

	// Manually set very short durations
	timer.mu.Lock()
	timer.state = StateWork
	timer.startedAt = time.Now()
	timer.endAt = time.Now().Add(50 * time.Millisecond)
	stopCh := make(chan struct{})
	timer.stopCh = stopCh
	timer.mu.Unlock()

	go timer.runPhase(StateWork, stopCh)

	// Wait for transition
	time.Sleep(200 * time.Millisecond)

	state := timer.State()
	// Should be in break or done (break duration is also 0)
	if state != StateBreak && state != StateDone {
		t.Errorf("expected break or done after work phase, got %s", state)
	}

	timer.Cancel()
}

func TestTimer_RemainingDecrements(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	timer.Start(nil)

	rem := timer.Remaining()
	if rem > 25*time.Minute+time.Second || rem < 24*time.Minute {
		t.Errorf("expected ~25m remaining, got %v", rem)
	}

	timer.Cancel()
}

func TestTimer_TagAssociation(t *testing.T) {
	timer := NewTimer(DefaultConfig())
	tagID := int64(42)
	timer.Start(&tagID)

	select {
	case sc := <-timer.Changes():
		if sc.TagID == nil || *sc.TagID != 42 {
			t.Errorf("expected tagID 42, got %v", sc.TagID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected state change")
	}

	timer.Cancel()
}
```

- [ ] **5.3** Run tests

```bash
go test ./internal/client/pomodoro/ -v -run TestTimer
# Expected: all tests PASS
```

- [ ] **5.4** Commit

```bash
mkdir -p internal/client/pomodoro
git add internal/client/pomodoro/timer.go internal/client/pomodoro/timer_test.go
git commit -m "feat(pomodoro): add timer state machine with work/break cycle and tag association"
```

---

## Task 6: Pomodoro Store

**Files:**
- Create: `internal/client/store/pomodoro.go`
- Create: `internal/client/store/pomodoro_test.go`

### Steps

- [ ] **6.1** Create the pomodoro sessions store

```go
// internal/client/store/pomodoro.go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// PomodoroSession represents a row in pomodoro_sessions.
type PomodoroSession struct {
	ID        int64
	StartedAt time.Time
	EndedAt   *time.Time
	WorkMins  int
	BreakMins int
	Status    string // "work", "break", "done", "cancelled"
	TagID     *int64
	CreatedAt time.Time
}

// PomodoroStore handles CRUD for pomodoro_sessions.
type PomodoroStore struct {
	db *sql.DB
}

// NewPomodoroStore creates a new pomodoro store.
func NewPomodoroStore(db *sql.DB) *PomodoroStore {
	return &PomodoroStore{db: db}
}

// Create inserts a new pomodoro session. Returns the new ID.
func (s *PomodoroStore) Create(workMins, breakMins int, status string, tagID *int64) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(
		`INSERT INTO pomodoro_sessions (started_at, work_mins, break_mins, status, tag_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		now, workMins, breakMins, status, tagID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("pomodoro store: create: %w", err)
	}
	return result.LastInsertId()
}

// UpdateStatus updates the status of a pomodoro session.
func (s *PomodoroStore) UpdateStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE pomodoro_sessions SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("pomodoro store: update status: %w", err)
	}
	return nil
}

// End marks a session as ended with the given status.
func (s *PomodoroStore) End(id int64, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE pomodoro_sessions SET ended_at = ?, status = ? WHERE id = ?`,
		now, status, id,
	)
	if err != nil {
		return fmt.Errorf("pomodoro store: end: %w", err)
	}
	return nil
}

// GetByID fetches a single pomodoro session.
func (s *PomodoroStore) GetByID(id int64) (*PomodoroSession, error) {
	row := s.db.QueryRow(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions WHERE id = ?`, id,
	)
	return scanPomodoroSession(row)
}

// ListByDate returns pomodoro sessions for a given date (UTC).
func (s *PomodoroStore) ListByDate(date time.Time) ([]PomodoroSession, error) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	dayEnd := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, time.UTC).Format(time.RFC3339)

	rows, err := s.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions
		 WHERE started_at >= ? AND started_at <= ?
		 ORDER BY started_at DESC`, dayStart, dayEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("pomodoro store: list by date: %w", err)
	}
	defer rows.Close()

	var sessions []PomodoroSession
	for rows.Next() {
		ps, err := scanPomodoroSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, ps)
	}
	return sessions, rows.Err()
}

// ListRecent returns the N most recent pomodoro sessions.
func (s *PomodoroStore) ListRecent(limit int) ([]PomodoroSession, error) {
	rows, err := s.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id, created_at
		 FROM pomodoro_sessions
		 ORDER BY started_at DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pomodoro store: list recent: %w", err)
	}
	defer rows.Close()

	var sessions []PomodoroSession
	for rows.Next() {
		ps, err := scanPomodoroSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, ps)
	}
	return sessions, rows.Err()
}

type pomodoroScanner interface {
	Scan(dest ...any) error
}

func scanPomodoroFromScanner(s pomodoroScanner) (PomodoroSession, error) {
	var ps PomodoroSession
	var startedAt, createdAt string
	var endedAt sql.NullString
	var tagID sql.NullInt64

	err := s.Scan(&ps.ID, &startedAt, &endedAt, &ps.WorkMins, &ps.BreakMins,
		&ps.Status, &tagID, &createdAt)
	if err != nil {
		return ps, fmt.Errorf("pomodoro store: scan: %w", err)
	}

	ps.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	ps.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		ps.EndedAt = &t
	}
	if tagID.Valid {
		v := tagID.Int64
		ps.TagID = &v
	}
	return ps, nil
}

func scanPomodoroSession(row *sql.Row) (*PomodoroSession, error) {
	ps, err := scanPomodoroFromScanner(row)
	if err != nil {
		return nil, err
	}
	return &ps, nil
}

func scanPomodoroSessionRow(rows *sql.Rows) (PomodoroSession, error) {
	return scanPomodoroFromScanner(rows)
}
```

- [ ] **6.2** Write tests

```go
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
```

- [ ] **6.3** Run tests

```bash
go test ./internal/client/store/ -v -run TestPomodoroStore
# Expected: all tests PASS
```

- [ ] **6.4** Commit

```bash
git add internal/client/store/pomodoro.go internal/client/store/pomodoro_test.go
git commit -m "feat(store): add pomodoro session CRUD with date queries"
```

---

## Task 7: System Tray

**Files:**
- Create: `internal/client/tray/tray.go`

### Steps

- [ ] **7.1** Add systray dependency

```bash
go get github.com/getlantern/systray
```

- [ ] **7.2** Create the system tray implementation

```go
// internal/client/tray/tray.go
package tray

import (
	"fmt"

	"github.com/getlantern/systray"
)

// Actions is the interface the tray uses to trigger application behaviors.
type Actions interface {
	OpenDashboard()
	StartPomodoro()
	StopPomodoro()
	IsTrackingOn() bool
	SetTrackingOn(on bool)
	IsAutostartOn() bool
	SetAutostartOn(on bool)
	Quit()
}

// PomodoroInfo provides current pomodoro state for tray display.
type PomodoroInfo struct {
	Active    bool
	State     string // "work", "break", "idle"
	Remaining string // e.g., "18:32"
}

// Tray manages the system tray icon and menu.
type Tray struct {
	actions  Actions
	menuDash *systray.MenuItem
	menuPomo *systray.MenuItem
	menuTrack *systray.MenuItem
	menuAuto  *systray.MenuItem
	menuQuit  *systray.MenuItem
}

// New creates a Tray. Call Run() to start it (blocks until quit).
func New(actions Actions) *Tray {
	return &Tray{actions: actions}
}

// Run starts the system tray. This blocks until the tray exits.
// Must be called from the main goroutine on macOS.
func (t *Tray) Run() {
	systray.Run(t.onReady, t.onExit)
}

// Quit requests the tray to close.
func (t *Tray) Quit() {
	systray.Quit()
}

func (t *Tray) onReady() {
	systray.SetTitle("Trasker")
	systray.SetTooltip("Trasker — Time Tracking")

	t.menuDash = systray.AddMenuItem("Open Dashboard...", "Open browser dashboard")
	t.menuPomo = systray.AddMenuItem("Start Pomodoro", "25m / 5m break")
	systray.AddSeparator()

	t.menuTrack = systray.AddMenuItemCheckbox("Tracking ON", "Toggle tracking", t.actions.IsTrackingOn())
	t.menuAuto = systray.AddMenuItemCheckbox("Start with OS", "Toggle autostart", t.actions.IsAutostartOn())
	systray.AddSeparator()

	t.menuQuit = systray.AddMenuItem("Quit Trasker", "Exit application")

	go t.eventLoop()
}

func (t *Tray) onExit() {
	// Cleanup if needed
}

func (t *Tray) eventLoop() {
	for {
		select {
		case <-t.menuDash.ClickedCh:
			t.actions.OpenDashboard()

		case <-t.menuPomo.ClickedCh:
			t.actions.StartPomodoro()

		case <-t.menuTrack.ClickedCh:
			newState := !t.actions.IsTrackingOn()
			t.actions.SetTrackingOn(newState)
			if newState {
				t.menuTrack.Check()
				t.menuTrack.SetTitle("Tracking ON")
			} else {
				t.menuTrack.Uncheck()
				t.menuTrack.SetTitle("Tracking OFF")
			}

		case <-t.menuAuto.ClickedCh:
			newState := !t.actions.IsAutostartOn()
			t.actions.SetAutostartOn(newState)
			if newState {
				t.menuAuto.Check()
			} else {
				t.menuAuto.Uncheck()
			}

		case <-t.menuQuit.ClickedCh:
			t.actions.Quit()
			systray.Quit()
			return
		}
	}
}

// UpdateTracking updates the tray display to reflect tracking state.
func (t *Tray) UpdateTracking(on bool) {
	if t.menuTrack == nil {
		return
	}
	if on {
		t.menuTrack.Check()
		t.menuTrack.SetTitle("Tracking ON")
		systray.SetTooltip("Trasker — Tracking")
	} else {
		t.menuTrack.Uncheck()
		t.menuTrack.SetTitle("Tracking OFF")
		systray.SetTooltip("Trasker — Paused")
	}
}

// UpdatePomodoro updates the tray to show pomodoro state.
func (t *Tray) UpdatePomodoro(info PomodoroInfo) {
	if t.menuPomo == nil {
		return
	}
	if info.Active {
		t.menuPomo.SetTitle(fmt.Sprintf("Pomodoro: %s (%s)", info.Remaining, info.State))
		systray.SetTitle(fmt.Sprintf("⏱ %s", info.Remaining))
	} else {
		t.menuPomo.SetTitle("Start Pomodoro")
		systray.SetTitle("Trasker")
	}
}
```

- [ ] **7.3** Commit (no unit tests — systray requires a display server; tested via integration)

```bash
git add internal/client/tray/tray.go
git commit -m "feat(tray): add system tray with menu for dashboard, pomodoro, tracking, autostart, quit"
```

---

## Task 8: Notifications — Interface

**Files:**
- Create: `internal/client/notify/notify.go`

### Steps

- [ ] **8.1** Create the notification interface

```go
// internal/client/notify/notify.go
package notify

// ClickAction is a callback invoked when the user clicks a notification.
type ClickAction func()

// Notifier sends OS-native notifications.
type Notifier interface {
	// Notify sends a notification. onClick is called if the user clicks it (may be nil).
	Notify(title, body string, onClick ClickAction) error

	// Close cleans up resources.
	Close() error
}
```

- [ ] **8.2** Commit

```bash
git add internal/client/notify/notify.go
git commit -m "feat(notify): add Notifier interface for OS-native notifications"
```

---

## Task 9: Notifications — Linux

**Files:**
- Create: `internal/client/notify/notify_linux.go`
- Create: `internal/client/notify/notify_linux_test.go`

### Steps

- [ ] **9.1** Create Linux notification implementation using DBus

```go
// internal/client/notify/notify_linux.go
//go:build linux

package notify

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// LinuxNotifier sends notifications via org.freedesktop.Notifications (libnotify/DBus).
type LinuxNotifier struct {
	conn      *dbus.Conn
	mu        sync.Mutex
	callbacks map[uint32]ClickAction
	nextID    uint32
}

// NewLinuxNotifier creates a new Linux notifier using the session DBus.
func NewLinuxNotifier() (*LinuxNotifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("notify linux: connect dbus: %w", err)
	}

	n := &LinuxNotifier{
		conn:      conn,
		callbacks: make(map[uint32]ClickAction),
	}

	// Listen for ActionInvoked signals
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/Notifications"),
		dbus.WithMatchInterface("org.freedesktop.Notifications"),
		dbus.WithMatchMember("ActionInvoked"),
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("notify linux: add match: %w", err)
	}

	go n.listenSignals()

	return n, nil
}

// Notify sends a desktop notification.
func (n *LinuxNotifier) Notify(title, body string, onClick ClickAction) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	obj := n.conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")

	actions := []string{}
	if onClick != nil {
		actions = []string{"default", "Open"}
	}

	call := obj.Call("org.freedesktop.Notifications.Notify", 0,
		"trasker",       // app_name
		uint32(0),       // replaces_id
		"",              // app_icon
		title,           // summary
		body,            // body
		actions,         // actions
		map[string]dbus.Variant{}, // hints
		int32(10000),    // expire_timeout (ms), -1 = server default
	)
	if call.Err != nil {
		return fmt.Errorf("notify linux: send: %w", call.Err)
	}

	var id uint32
	if err := call.Store(&id); err != nil {
		return fmt.Errorf("notify linux: store id: %w", err)
	}

	if onClick != nil {
		n.callbacks[id] = onClick
	}

	return nil
}

// Close disconnects from DBus.
func (n *LinuxNotifier) Close() error {
	return n.conn.Close()
}

// listenSignals handles ActionInvoked signals from the notification server.
func (n *LinuxNotifier) listenSignals() {
	ch := make(chan *dbus.Signal, 16)
	n.conn.Signal(ch)

	for sig := range ch {
		if sig.Name != "org.freedesktop.Notifications.ActionInvoked" {
			continue
		}
		if len(sig.Body) < 2 {
			continue
		}

		id, ok := sig.Body[0].(uint32)
		if !ok {
			continue
		}

		n.mu.Lock()
		cb, exists := n.callbacks[id]
		if exists {
			delete(n.callbacks, id)
		}
		n.mu.Unlock()

		if exists && cb != nil {
			go cb()
		}
	}
}
```

- [ ] **9.2** Add dbus dependency

```bash
go get github.com/godbus/dbus/v5
```

- [ ] **9.3** Write a build verification test (actual notification delivery requires a display server)

```go
// internal/client/notify/notify_linux_test.go
//go:build linux

package notify

import (
	"testing"
)

func TestLinuxNotifier_ImplementsInterface(t *testing.T) {
	// Compile-time check that LinuxNotifier satisfies Notifier
	var _ Notifier = (*LinuxNotifier)(nil)
}
```

- [ ] **9.4** Run test

```bash
go test ./internal/client/notify/ -v -run TestLinuxNotifier
# Expected: PASS (compile-time interface check)
```

- [ ] **9.5** Commit

```bash
git add internal/client/notify/notify.go internal/client/notify/notify_linux.go internal/client/notify/notify_linux_test.go
git commit -m "feat(notify): add Linux notification via DBus org.freedesktop.Notifications"
```

---

## Task 10: Notifications — Integration

**Files:**
- Create: `internal/client/notify/dispatcher.go`
- Create: `internal/client/notify/dispatcher_test.go`

### Steps

- [ ] **10.1** Create the notification dispatcher that wires notifications to system events

```go
// internal/client/notify/dispatcher.go
package notify

import (
	"fmt"
	"log/slog"
	"time"
)

// Event types that trigger notifications.
type EventType int

const (
	EventDeadman       EventType = iota // "Still there?" presence check
	EventLongFocus                      // 1hr+ on same window
	EventTrackingOff                    // Tracking is paused reminder
	EventPomodoroWork                   // Break finished, back to work
	EventPomodoroBreak                  // Work finished, take a break
	EventPomodoroDone                   // Full cycle complete
)

// Dispatcher routes application events to OS notifications.
type Dispatcher struct {
	notifier Notifier
	logger   *slog.Logger
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(notifier Notifier, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		notifier: notifier,
		logger:   logger,
	}
}

// DeadmanCheck sends a "Still there?" notification with a countdown.
// Returns a channel that receives true if clicked, false if expired.
func (d *Dispatcher) DeadmanCheck(timeout time.Duration) chan bool {
	result := make(chan bool, 1)
	clicked := false

	onClick := func() {
		clicked = true
		result <- true
	}

	err := d.notifier.Notify(
		"Trasker — Still there?",
		fmt.Sprintf("Click within %d seconds to confirm you're active.", int(timeout.Seconds())),
		onClick,
	)
	if err != nil {
		d.logger.Error("failed to send deadman notification", "error", err)
		result <- false
		return result
	}

	go func() {
		time.Sleep(timeout)
		if !clicked {
			result <- false
		}
	}()

	return result
}

// LongFocusReminder notifies about extended focus on one window.
func (d *Dispatcher) LongFocusReminder(appName string, duration time.Duration) {
	hours := int(duration.Hours())
	mins := int(duration.Minutes()) % 60
	body := fmt.Sprintf("You've been focused on %s for %dh%dm.", appName, hours, mins)

	if err := d.notifier.Notify("Trasker — Long Focus", body, nil); err != nil {
		d.logger.Error("failed to send long focus notification", "error", err)
	}
}

// TrackingOffNag sends a gentle reminder that tracking is paused.
func (d *Dispatcher) TrackingOffNag(onResume ClickAction) {
	if err := d.notifier.Notify(
		"Trasker is paused",
		"Click to resume tracking.",
		onResume,
	); err != nil {
		d.logger.Error("failed to send tracking-off nag", "error", err)
	}
}

// PomodoroTransition notifies about pomodoro state changes.
func (d *Dispatcher) PomodoroTransition(eventType EventType) {
	var title, body string
	switch eventType {
	case EventPomodoroBreak:
		title = "Trasker — Break Time"
		body = "Work phase complete. Take a break!"
	case EventPomodoroWork:
		title = "Trasker — Back to Work"
		body = "Break is over. Time to focus!"
	case EventPomodoroDone:
		title = "Trasker — Pomodoro Complete"
		body = "Full pomodoro cycle finished."
	default:
		return
	}

	if err := d.notifier.Notify(title, body, nil); err != nil {
		d.logger.Error("failed to send pomodoro notification", "error", err)
	}
}
```

- [ ] **10.2** Write tests with a mock notifier

```go
// internal/client/notify/dispatcher_test.go
package notify

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockNotifier records all notifications for testing.
type mockNotifier struct {
	mu            sync.Mutex
	notifications []mockNotification
}

type mockNotification struct {
	Title   string
	Body    string
	HasClick bool
}

func (m *mockNotifier) Notify(title, body string, onClick ClickAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, mockNotification{
		Title:    title,
		Body:     body,
		HasClick: onClick != nil,
	})
	// Simulate user click if handler provided
	if onClick != nil {
		go onClick()
	}
	return nil
}

func (m *mockNotifier) Close() error { return nil }

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func (m *mockNotifier) last() mockNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifications[len(m.notifications)-1]
}

func TestDispatcher_DeadmanCheck_Clicked(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	result := d.DeadmanCheck(5 * time.Second)

	// Mock auto-clicks, so we should get true
	select {
	case clicked := <-result:
		if !clicked {
			t.Error("expected clicked=true")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected result")
	}

	if mock.count() != 1 {
		t.Errorf("expected 1 notification, got %d", mock.count())
	}
}

func TestDispatcher_TrackingOffNag(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	resumed := false
	d.TrackingOffNag(func() { resumed = true })

	time.Sleep(50 * time.Millisecond) // let goroutine run

	if !resumed {
		t.Error("expected resume callback to fire")
	}
	n := mock.last()
	if n.Title != "Trasker is paused" {
		t.Errorf("unexpected title: %q", n.Title)
	}
}

func TestDispatcher_PomodoroTransitions(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	d.PomodoroTransition(EventPomodoroBreak)
	d.PomodoroTransition(EventPomodoroWork)
	d.PomodoroTransition(EventPomodoroDone)

	if mock.count() != 3 {
		t.Errorf("expected 3 notifications, got %d", mock.count())
	}
}

func TestDispatcher_LongFocusReminder(t *testing.T) {
	mock := &mockNotifier{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	d := NewDispatcher(mock, logger)

	d.LongFocusReminder("VS Code", 90*time.Minute)

	if mock.count() != 1 {
		t.Errorf("expected 1 notification, got %d", mock.count())
	}
	n := mock.last()
	if n.Title != "Trasker — Long Focus" {
		t.Errorf("unexpected title: %q", n.Title)
	}
}
```

- [ ] **10.3** Run tests

```bash
go test ./internal/client/notify/ -v -run TestDispatcher
# Expected: all tests PASS
```

- [ ] **10.4** Commit

```bash
git add internal/client/notify/dispatcher.go internal/client/notify/dispatcher_test.go
git commit -m "feat(notify): add dispatcher routing deadman, pomodoro, tracking-off to OS notifications"
```

---

## Task 11: Sync Client

**Files:**
- Create: `internal/client/sync/client.go`
- Create: `internal/client/sync/client_test.go`

### Steps

- [ ] **11.1** Create the HTTP sync client for server communication

```go
// internal/client/sync/client.go
package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeviceRegistration is the payload for POST /api/v1/devices.
type DeviceRegistration struct {
	ClientDeviceID string `json:"client_device_id"`
	OS             string `json:"os"`
	Hostname       string `json:"hostname"`
}

// TimesheetEntry is a single entry in a timesheet submission.
type TimesheetEntry struct {
	Tag        string `json:"tag"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	DurationS  int    `json:"duration_s"`
	Notes      string `json:"notes,omitempty"`
	AppSummary string `json:"app_summary,omitempty"`
}

// TimesheetSubmission is the payload for POST /api/v1/timesheets.
type TimesheetSubmission struct {
	ClientDeviceID string           `json:"client_device_id"`
	Entries        []TimesheetEntry `json:"entries"`
}

// TimesheetResponse is returned by the server after submission.
type TimesheetResponse struct {
	ID          string `json:"id"`
	SubmittedAt string `json:"submitted_at"`
}

// ErrorResponse represents a server error.
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ErrorResponse) Error() string {
	return fmt.Sprintf("server error %d: %s", e.Code, e.Message)
}

// ErrKeyExpired indicates the API key has expired.
var ErrKeyExpired = fmt.Errorf("API key expired — download a new client from your Trasker dashboard")

// ErrKeyRevoked indicates the API key has been revoked.
var ErrKeyRevoked = fmt.Errorf("API key revoked — contact your administrator")

// Client handles HTTP communication with the Trasker server.
type Client struct {
	httpClient *http.Client
	serverURL  string
	apiKey     string
}

// NewClient creates a sync client with the given server URL and API key.
func NewClient(serverURL, apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		serverURL:  serverURL,
		apiKey:     apiKey,
	}
}

// RegisterDevice registers this device with the server. Idempotent (server upserts).
func (c *Client) RegisterDevice(ctx context.Context, reg DeviceRegistration) error {
	_, err := c.post(ctx, "/api/v1/devices", reg)
	return err
}

// SubmitTimesheet sends a timesheet to the server. Returns the server-assigned ID.
func (c *Client) SubmitTimesheet(ctx context.Context, sub TimesheetSubmission) (*TimesheetResponse, error) {
	body, err := c.post(ctx, "/api/v1/timesheets", sub)
	if err != nil {
		return nil, err
	}

	var resp TimesheetResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sync client: unmarshal response: %w", err)
	}
	return &resp, nil
}

// HealthCheck pings the server health endpoint.
func (c *Client) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("sync client: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync client: health check: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync client: health check returned %d", resp.StatusCode)
	}
	return nil
}

// post sends a POST request and handles common error responses.
func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sync client: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("sync client: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sync client: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sync client: read body: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return body, nil
	case http.StatusUnauthorized:
		return nil, ErrKeyExpired
	case http.StatusForbidden:
		return nil, ErrKeyRevoked
	default:
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
			return nil, &errResp
		}
		return nil, fmt.Errorf("sync client: server returned %d: %s", resp.StatusCode, string(body))
	}
}
```

- [ ] **11.2** Write tests using httptest

```go
// internal/client/sync/client_test.go
package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_RegisterDevice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/devices" {
			t.Errorf("expected /api/v1/devices, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong auth header")
		}

		var reg DeviceRegistration
		json.NewDecoder(r.Body).Decode(&reg)
		if reg.ClientDeviceID != "dev-123" {
			t.Errorf("expected device id 'dev-123', got %q", reg.ClientDeviceID)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{
		ClientDeviceID: "dev-123",
		OS:             "linux",
		Hostname:       "workstation",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestClient_SubmitTimesheet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/timesheets" {
			t.Errorf("expected /api/v1/timesheets, got %s", r.URL.Path)
		}

		var sub TimesheetSubmission
		json.NewDecoder(r.Body).Decode(&sub)
		if len(sub.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(sub.Entries))
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{
			ID:          "server-id-abc",
			SubmittedAt: "2026-03-23T12:00:00Z",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resp, err := client.SubmitTimesheet(context.Background(), TimesheetSubmission{
		ClientDeviceID: "dev-123",
		Entries: []TimesheetEntry{
			{Tag: "Dev", StartedAt: "2026-03-23T09:00:00Z", EndedAt: "2026-03-23T12:00:00Z", DurationS: 10800},
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if resp.ID != "server-id-abc" {
		t.Errorf("expected id 'server-id-abc', got %q", resp.ID)
	}
}

func TestClient_KeyExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "expired-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err != ErrKeyExpired {
		t.Errorf("expected ErrKeyExpired, got %v", err)
	}
}

func TestClient_KeyRevoked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "revoked-key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err != ErrKeyRevoked {
		t.Errorf("expected ErrKeyRevoked, got %v", err)
	}
}

func TestClient_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Code: 500, Message: "internal error"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	err := client.RegisterDevice(context.Background(), DeviceRegistration{ClientDeviceID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("expected *ErrorResponse, got %T", err)
	}
	if errResp.Code != 500 {
		t.Errorf("expected code 500, got %d", errResp.Code)
	}
}

func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("expected /api/v1/health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
}
```

- [ ] **11.3** Run tests

```bash
go test ./internal/client/sync/ -v -run TestClient
# Expected: all tests PASS
```

- [ ] **11.4** Commit

```bash
git add internal/client/sync/client.go internal/client/sync/client_test.go
git commit -m "feat(sync): add HTTP client for server API with auth error handling"
```

---

## Task 12: Submission Queue

**Files:**
- Create: `internal/client/sync/queue.go`
- Create: `internal/client/sync/queue_test.go`

### Steps

- [ ] **12.1** Create the submission queue with exponential backoff retry

```go
// internal/client/sync/queue.go
package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	gosync "sync"
	"time"
)

// Backoff schedule for retries: 1m, 5m, 15m, 1h, then hourly.
var backoffSchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	1 * time.Hour,
}

// PendingSubmission represents a locally queued submission awaiting sync.
type PendingSubmission struct {
	ID         int64
	Status     string
	RetryCount int
	LastRetry  *time.Time
	CreatedAt  time.Time
}

// Queue processes pending submissions from the local store.
type Queue struct {
	db       *sql.DB
	client   *Client
	deviceID string
	logger   *slog.Logger
	mu       gosync.Mutex
	running  bool
	stopCh   chan struct{}
}

// NewQueue creates a submission queue.
func NewQueue(db *sql.DB, client *Client, deviceID string, logger *slog.Logger) *Queue {
	return &Queue{
		db:       db,
		client:   client,
		deviceID: deviceID,
		logger:   logger,
	}
}

// Start begins periodic processing of the submission queue.
// Processes immediately on start, then every 5 minutes.
func (q *Queue) Start(ctx context.Context) {
	q.mu.Lock()
	if q.running {
		q.mu.Unlock()
		return
	}
	q.running = true
	q.stopCh = make(chan struct{})
	q.mu.Unlock()

	go q.run(ctx)
}

// Stop halts queue processing.
func (q *Queue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running {
		close(q.stopCh)
		q.running = false
	}
}

func (q *Queue) run(ctx context.Context) {
	// Process immediately
	q.ProcessPending(ctx)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.ProcessPending(ctx)
		}
	}
}

// ProcessPending attempts to submit all pending entries that are due for retry.
func (q *Queue) ProcessPending(ctx context.Context) {
	pending, err := q.listDueSubmissions()
	if err != nil {
		q.logger.Error("failed to list pending submissions", "error", err)
		return
	}

	for _, sub := range pending {
		if ctx.Err() != nil {
			return
		}
		q.processOne(ctx, sub)
	}
}

// listDueSubmissions returns pending submissions that are ready for retry.
func (q *Queue) listDueSubmissions() ([]PendingSubmission, error) {
	rows, err := q.db.Query(
		`SELECT id, status, retry_count, last_retry
		 FROM submissions
		 WHERE status = 'pending'
		 ORDER BY submitted_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: list pending: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var due []PendingSubmission
	for rows.Next() {
		var sub PendingSubmission
		var lastRetry sql.NullString
		if err := rows.Scan(&sub.ID, &sub.Status, &sub.RetryCount, &lastRetry); err != nil {
			return nil, fmt.Errorf("queue: scan: %w", err)
		}
		if lastRetry.Valid {
			t, _ := time.Parse(time.RFC3339, lastRetry.String)
			sub.LastRetry = &t
		}

		// Check if enough time has passed since last retry
		if sub.LastRetry != nil {
			backoff := q.backoffFor(sub.RetryCount)
			if now.Before(sub.LastRetry.Add(backoff)) {
				continue // not due yet
			}
		}

		due = append(due, sub)
	}
	return due, rows.Err()
}

// backoffFor returns the backoff duration for the given retry count.
func (q *Queue) backoffFor(retryCount int) time.Duration {
	if retryCount < len(backoffSchedule) {
		return backoffSchedule[retryCount]
	}
	return 1 * time.Hour // cap at hourly
}

// processOne attempts to submit a single pending submission.
func (q *Queue) processOne(ctx context.Context, sub PendingSubmission) {
	// Load the submission's entries from the store
	entries, err := q.loadSubmissionEntries(sub.ID)
	if err != nil {
		q.logger.Error("failed to load submission entries", "submission_id", sub.ID, "error", err)
		return
	}

	if len(entries) == 0 {
		q.logger.Warn("submission has no entries, marking confirmed", "submission_id", sub.ID)
		q.updateStatus(sub.ID, "confirmed", nil)
		return
	}

	// Aggregate short focus events before submission (per spec)
	entries = Aggregate(entries)

	resp, err := q.client.SubmitTimesheet(ctx, TimesheetSubmission{
		ClientDeviceID: q.deviceID,
		Entries:        entries,
	})
	if err != nil {
		// Handle permanent errors (don't retry)
		if err == ErrKeyExpired || err == ErrKeyRevoked {
			q.logger.Error("permanent sync error", "submission_id", sub.ID, "error", err)
			return
		}

		// Transient error — bump retry
		q.logger.Warn("submission failed, will retry",
			"submission_id", sub.ID,
			"retry_count", sub.RetryCount+1,
			"error", err,
		)
		q.bumpRetry(sub.ID, sub.RetryCount)
		return
	}

	// Success
	q.updateStatus(sub.ID, "confirmed", &resp.ID)
	q.logger.Info("submission confirmed",
		"submission_id", sub.ID,
		"server_id", resp.ID,
	)
}

// loadSubmissionEntries builds TimesheetEntry payloads for a submission.
func (q *Queue) loadSubmissionEntries(submissionID int64) ([]TimesheetEntry, error) {
	rows, err := q.db.Query(
		`SELECT fe.app_name, fe.started_at, fe.ended_at, fe.duration_s,
		        COALESCE(t.name, 'Untagged') as tag,
		        COALESCE(n.text, '') as note
		 FROM submission_events se
		 JOIN focus_events fe ON fe.id = se.event_id
		 LEFT JOIN event_tags et ON et.event_id = fe.id
		 LEFT JOIN tags t ON t.id = et.tag_id
		 LEFT JOIN notes n ON n.anchor_event = fe.id
		 WHERE se.submission_id = ?
		 ORDER BY fe.started_at`, submissionID,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: load entries: %w", err)
	}
	defer rows.Close()

	var entries []TimesheetEntry
	for rows.Next() {
		var appName, startedAt, tag, note string
		var endedAt sql.NullString
		var durationS sql.NullInt64

		if err := rows.Scan(&appName, &startedAt, &endedAt, &durationS, &tag, &note); err != nil {
			return nil, fmt.Errorf("queue: scan entry: %w", err)
		}

		end := ""
		if endedAt.Valid {
			end = endedAt.String
		}
		dur := 0
		if durationS.Valid {
			dur = int(durationS.Int64)
		}

		entries = append(entries, TimesheetEntry{
			Tag:       tag,
			StartedAt: startedAt,
			EndedAt:   end,
			DurationS: dur,
			Notes:     note,
		})
	}
	return entries, rows.Err()
}

func (q *Queue) updateStatus(id int64, status string, serverID *string) {
	_, err := q.db.Exec(
		`UPDATE submissions SET status = ?, server_id = ? WHERE id = ?`,
		status, serverID, id,
	)
	if err != nil {
		q.logger.Error("failed to update submission status", "id", id, "error", err)
	}
}

func (q *Queue) bumpRetry(id int64, currentCount int) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := q.db.Exec(
		`UPDATE submissions SET retry_count = ?, last_retry = ? WHERE id = ?`,
		currentCount+1, now, id,
	)
	if err != nil {
		q.logger.Error("failed to bump retry", "id", id, "error", err)
	}
}
```

- [ ] **12.2** Write tests

```go
// internal/client/sync/queue_test.go
package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupQueueDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT, created_at TEXT)`,
		`CREATE TABLE focus_events (
			id INTEGER PRIMARY KEY, app_name TEXT NOT NULL, window_title TEXT NOT NULL,
			started_at TEXT NOT NULL, ended_at TEXT, duration_s INTEGER,
			is_idle INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE event_tags (event_id INTEGER, tag_id INTEGER, source TEXT, cascade_from INTEGER,
			UNIQUE(event_id, tag_id))`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, anchor_event INTEGER, text TEXT, created_at TEXT,
			cascade_applied INTEGER DEFAULT 0)`,
		`CREATE TABLE submissions (
			id INTEGER PRIMARY KEY, server_id TEXT, submitted_at TEXT NOT NULL,
			status TEXT NOT NULL, retry_count INTEGER NOT NULL DEFAULT 0, last_retry TEXT)`,
		`CREATE TABLE submission_events (submission_id INTEGER, event_id INTEGER,
			UNIQUE(submission_id, event_id))`,
		// Seed data
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Dev', '#00ff00', '2026-01-01T00:00:00Z')`,
		`INSERT INTO focus_events (id, app_name, window_title, started_at, ended_at, duration_s, created_at)
		 VALUES (1, 'terminal', 'claude', '2026-03-23T09:00:00Z', '2026-03-23T10:00:00Z', 3600, '2026-03-23T09:00:00Z')`,
		`INSERT INTO event_tags (event_id, tag_id, source) VALUES (1, 1, 'manual')`,
		`INSERT INTO submissions (id, submitted_at, status) VALUES (1, '2026-03-23T12:00:00Z', 'pending')`,
		`INSERT INTO submission_events (submission_id, event_id) VALUES (1, 1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt[:40], err)
		}
	}
	return db
}

func TestQueue_ProcessPending_Success(t *testing.T) {
	db := setupQueueDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{ID: "srv-1", SubmittedAt: "2026-03-23T12:00:00Z"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	// Check submission was confirmed
	var status string
	var serverID sql.NullString
	db.QueryRow(`SELECT status, server_id FROM submissions WHERE id = 1`).Scan(&status, &serverID)
	if status != "confirmed" {
		t.Errorf("expected 'confirmed', got %q", status)
	}
	if !serverID.Valid || serverID.String != "srv-1" {
		t.Errorf("expected server_id 'srv-1', got %v", serverID)
	}
}

func TestQueue_ProcessPending_RetryOnError(t *testing.T) {
	db := setupQueueDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	var retryCount int
	var lastRetry sql.NullString
	db.QueryRow(`SELECT retry_count, last_retry FROM submissions WHERE id = 1`).Scan(&retryCount, &lastRetry)
	if retryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", retryCount)
	}
	if !lastRetry.Valid {
		t.Error("expected last_retry to be set")
	}
}

func TestQueue_BackoffSchedule(t *testing.T) {
	queue := &Queue{}

	tests := []struct {
		retry   int
		expect  time.Duration
	}{
		{0, 1 * time.Minute},
		{1, 5 * time.Minute},
		{2, 15 * time.Minute},
		{3, 1 * time.Hour},
		{4, 1 * time.Hour}, // capped
		{99, 1 * time.Hour},
	}

	for _, tt := range tests {
		got := queue.backoffFor(tt.retry)
		if got != tt.expect {
			t.Errorf("backoff(%d) = %v, want %v", tt.retry, got, tt.expect)
		}
	}
}

func TestQueue_SkipsNotDueSubmissions(t *testing.T) {
	db := setupQueueDB(t)
	// Mark as recently retried so it's not due
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec(`UPDATE submissions SET retry_count = 1, last_retry = ? WHERE id = 1`, now)

	serverCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TimesheetResponse{ID: "srv-1"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	queue := NewQueue(db, client, "dev-123", logger)

	queue.ProcessPending(context.Background())

	if serverCalled {
		t.Error("server should not have been called — submission not yet due")
	}
}
```

- [ ] **12.3** Run tests

```bash
go test ./internal/client/sync/ -v -run TestQueue
# Expected: all tests PASS
```

- [ ] **12.4** Commit

```bash
git add internal/client/sync/queue.go internal/client/sync/queue_test.go
git commit -m "feat(sync): add submission queue with exponential backoff retry"
```

---

## Task 13: Submit Flow

**Files:**
- Create: `internal/client/sync/submit.go`
- Create: `internal/client/sync/submit_test.go`

### Steps

- [ ] **13.1** Create the submit flow that aggregates focus events into timesheet entries

```go
// internal/client/sync/submit.go
package sync

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SubmitService handles the user-facing submission flow: aggregate events by tag
// into time blocks, concatenate notes, compute app_summary, create local submission record.
type SubmitService struct {
	db *sql.DB
}

// NewSubmitService creates a new submit service.
func NewSubmitService(db *sql.DB) *SubmitService {
	return &SubmitService{db: db}
}

// RawEvent is a focus event with its tag and note data for aggregation.
type RawEvent struct {
	ID          int64
	AppName     string
	StartedAt   time.Time
	EndedAt     time.Time
	DurationS   int
	Tag         string
	NoteText    string
	NoteTime    *time.Time
}

// AggregatedEntry is a time block ready for submission.
type AggregatedEntry struct {
	Tag        string
	StartedAt  time.Time
	EndedAt    time.Time
	DurationS  int
	Notes      string
	AppSummary string
	EventIDs   []int64
}

// LoadEventsForSubmission fetches focus events by ID with their tags and notes.
func (s *SubmitService) LoadEventsForSubmission(eventIDs []int64) ([]RawEvent, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(eventIDs))
	args := make([]any, len(eventIDs))
	for i, id := range eventIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT fe.id, fe.app_name, fe.started_at, fe.ended_at, fe.duration_s,
		        COALESCE(t.name, 'Untagged') as tag,
		        COALESCE(n.text, '') as note_text,
		        n.created_at as note_time
		 FROM focus_events fe
		 LEFT JOIN event_tags et ON et.event_id = fe.id
		 LEFT JOIN tags t ON t.id = et.tag_id
		 LEFT JOIN notes n ON n.anchor_event = fe.id
		 WHERE fe.id IN (%s)
		 ORDER BY fe.started_at`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("submit: load events: %w", err)
	}
	defer rows.Close()

	var events []RawEvent
	for rows.Next() {
		var e RawEvent
		var startedAt, endedAt string
		var noteTime sql.NullString

		if err := rows.Scan(&e.ID, &e.AppName, &startedAt, &endedAt, &e.DurationS,
			&e.Tag, &e.NoteText, &noteTime); err != nil {
			return nil, fmt.Errorf("submit: scan event: %w", err)
		}

		e.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		e.EndedAt, _ = time.Parse(time.RFC3339, endedAt)
		if noteTime.Valid {
			t, _ := time.Parse(time.RFC3339, noteTime.String)
			e.NoteTime = &t
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// Aggregate groups raw events by tag into time blocks with concatenated notes
// and app summary percentages.
func (s *SubmitService) Aggregate(events []RawEvent) []AggregatedEntry {
	// Group by tag
	groups := make(map[string][]RawEvent)
	for _, e := range events {
		groups[e.Tag] = append(groups[e.Tag], e)
	}

	var result []AggregatedEntry
	for tag, group := range groups {
		entry := aggregateGroup(tag, group)
		result = append(result, entry)
	}

	// Sort by start time
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.Before(result[j].StartedAt)
	})
	return result
}

func aggregateGroup(tag string, events []RawEvent) AggregatedEntry {
	// Sort by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartedAt.Before(events[j].StartedAt)
	})

	entry := AggregatedEntry{
		Tag:       tag,
		StartedAt: events[0].StartedAt,
		EndedAt:   events[len(events)-1].EndedAt,
	}

	// Sum duration, collect notes, tally app usage
	totalDuration := 0
	appDurations := make(map[string]int)
	var notes []string

	for _, e := range events {
		totalDuration += e.DurationS
		appDurations[e.AppName] += e.DurationS
		entry.EventIDs = append(entry.EventIDs, e.ID)

		if e.NoteText != "" && e.NoteTime != nil {
			timeStr := e.NoteTime.Format("15:04")
			notes = append(notes, fmt.Sprintf("[%s] %s", timeStr, e.NoteText))
		}
	}

	entry.DurationS = totalDuration
	entry.Notes = strings.Join(notes, "\n")
	entry.AppSummary = buildAppSummary(appDurations, totalDuration)

	return entry
}

// buildAppSummary generates "VS Code (72%), Terminal (18%), Firefox (10%)".
func buildAppSummary(appDurations map[string]int, totalDuration int) string {
	if totalDuration == 0 {
		return ""
	}

	type appPct struct {
		Name string
		Pct  int
	}

	var apps []appPct
	for name, dur := range appDurations {
		pct := (dur * 100) / totalDuration
		if pct > 0 {
			apps = append(apps, appPct{Name: name, Pct: pct})
		}
	}

	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Pct > apps[j].Pct
	})

	var parts []string
	for _, a := range apps {
		parts = append(parts, fmt.Sprintf("%s (%d%%)", a.Name, a.Pct))
	}
	return strings.Join(parts, ", ")
}

// CreateSubmission creates a local submission record and links events to it.
// Returns the submission ID.
func (s *SubmitService) CreateSubmission(eventIDs []int64) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("submit: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	result, err := tx.Exec(
		`INSERT INTO submissions (submitted_at, status, retry_count) VALUES (?, 'pending', 0)`,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("submit: insert submission: %w", err)
	}
	subID, _ := result.LastInsertId()

	stmt, err := tx.Prepare(`INSERT INTO submission_events (submission_id, event_id) VALUES (?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("submit: prepare: %w", err)
	}
	defer stmt.Close()

	for _, eid := range eventIDs {
		if _, err := stmt.Exec(subID, eid); err != nil {
			return 0, fmt.Errorf("submit: link event %d: %w", eid, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("submit: commit: %w", err)
	}
	return subID, nil
}
```

- [ ] **13.2** Write tests

```go
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
```

- [ ] **13.3** Run tests

```bash
go test ./internal/client/sync/ -v -run "TestAggregate|TestCreateSubmission"
# Expected: all tests PASS
```

- [ ] **13.4** Commit

```bash
git add internal/client/sync/submit.go internal/client/sync/submit_test.go
git commit -m "feat(sync): add submit flow with event aggregation, note concatenation, app summary"
```

---

## Task 14: Local Web UI — SvelteKit Init

**Files:**
- Create: `web/client-ui/` (SvelteKit project)

### Steps

- [ ] **14.1** Scaffold SvelteKit project

```bash
mkdir -p web/client-ui
cd web/client-ui
npx sv create . --template minimal --types ts
```

- [ ] **14.2** Add TailwindCSS

```bash
cd web/client-ui
npx sv add tailwindcss
```

- [ ] **14.3** Install adapter-static for embedding

```bash
cd web/client-ui
npm install -D @sveltejs/adapter-static
```

- [ ] **14.4** Configure adapter-static

```ts
// web/client-ui/svelte.config.js
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: 'index.html',  // SPA mode
			precompress: false,
			strict: false
		})
	}
};

export default config;
```

- [ ] **14.5** Create SPA layout with prerender disabled

```ts
// web/client-ui/src/routes/+layout.ts
export const prerender = false;
export const ssr = false;
```

- [ ] **14.6** Verify build

```bash
cd web/client-ui && npm run build
# Expected: Build completes, output in web/client-ui/build/
```

- [ ] **14.7** Commit

```bash
git add web/client-ui/
git commit -m "feat(webui): scaffold SvelteKit project with TypeScript, TailwindCSS, adapter-static"
```

---

## Task 15: Local Web UI — API Layer

**Files:**
- Create: `internal/client/webui/server.go`
- Create: `internal/client/webui/server_test.go`
- Create: `internal/client/webui/embed.go`

### Steps

- [ ] **15.1** Create the embed file for the SPA assets

```go
// internal/client/webui/embed.go
package webui

import "embed"

// Assets holds the built Svelte SPA files.
// The build step must run before Go compilation.
//
//go:embed all:static
var Assets embed.FS
```

Note: During development, the `static` directory will be symlinked or copied from `web/client-ui/build/`. For production, the Makefile will build the SPA first, copy to `internal/client/webui/static/`, then compile Go.

- [ ] **15.2** Create the HTTP server

```go
// internal/client/webui/server.go
package webui

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server serves the local web dashboard and REST API.
type Server struct {
	db       *sql.DB
	logger   *slog.Logger
	mux      *http.ServeMux
	srv      *http.Server
	port     int
	listener net.Listener
}

// NewServer creates a new webui server.
func NewServer(db *sql.DB, port int, logger *slog.Logger) *Server {
	s := &Server{
		db:     db,
		logger: logger,
		mux:    http.NewServeMux(),
		port:   port,
	}
	s.registerRoutes()
	return s
}

// Port returns the port the server is listening on.
// Useful when port 0 is used for testing.
func (s *Server) Port() int {
	if s.listener != nil {
		return s.listener.Addr().(*net.TCPAddr).Port
	}
	return s.port
}

// Start begins serving. Non-blocking.
func (s *Server) Start() error {
	var err error
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("webui: listen: %w", err)
	}

	s.srv = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("webui server error", "error", err)
		}
	}()

	s.logger.Info("webui server started", "addr", s.listener.Addr().String())
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// URL returns the full local URL to access the dashboard.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.Port())
}

func (s *Server) registerRoutes() {
	// Serve SPA static files
	staticFS, err := fs.Sub(Assets, "static")
	if err != nil {
		s.logger.Warn("webui: no embedded assets, SPA will not be served", "error", err)
	} else {
		s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}

	// API routes are registered in Task 16
}
```

- [ ] **15.3** Write test

```go
// internal/client/webui/server_test.go
package webui

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func setupWebDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestServer_StartAndStop(t *testing.T) {
	db := setupWebDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv := NewServer(db, 0, logger) // port 0 = random
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Stop(context.Background())

	// Server should be reachable
	resp, err := http.Get(srv.URL() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	// Expect 200 or 404 (no static files embedded in test)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("unexpected status: %d", resp.StatusCode)
	}
}
```

- [ ] **15.4** Run test

```bash
go test ./internal/client/webui/ -v -run TestServer
# Expected: PASS
```

- [ ] **15.5** Commit

```bash
git add internal/client/webui/server.go internal/client/webui/server_test.go internal/client/webui/embed.go
git commit -m "feat(webui): add localhost HTTP server with embedded SPA support"
```

---

## Task 16: Local Web UI — API Endpoints

**Files:**
- Create: `internal/client/webui/api.go`
- Create: `internal/client/webui/api_test.go`

### Steps

- [ ] **16.1** Create the REST API handler

```go
// internal/client/webui/api.go
package webui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// API provides REST endpoints for the local dashboard.
type API struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewAPI creates a new API handler set.
func NewAPI(db *sql.DB, logger *slog.Logger) *API {
	return &API{db: db, logger: logger}
}

// RegisterRoutes adds all API routes to the given mux.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", a.handleGetEvents)
	mux.HandleFunc("GET /api/tags", a.handleGetTags)
	mux.HandleFunc("POST /api/tags", a.handleCreateTag)
	mux.HandleFunc("POST /api/events/{id}/tag", a.handleTagEvent)
	mux.HandleFunc("POST /api/events/{id}/note", a.handleAddNote)
	mux.HandleFunc("GET /api/submissions", a.handleGetSubmissions)
	mux.HandleFunc("POST /api/submit", a.handleSubmit)
	mux.HandleFunc("GET /api/pomodoro", a.handleGetPomodoro)
	mux.HandleFunc("POST /api/pomodoro/start", a.handleStartPomodoro)
	mux.HandleFunc("GET /api/config", a.handleGetConfig)
	mux.HandleFunc("PATCH /api/config", a.handleUpdateConfig)
}

// --- Events ---

func (a *API) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	dayStart := date + "T00:00:00Z"
	dayEnd := date + "T23:59:59Z"

	rows, err := a.db.Query(
		`SELECT fe.id, fe.app_name, fe.window_title, fe.started_at, fe.ended_at,
		        fe.duration_s, fe.is_idle,
		        COALESCE(t.name, '') as tag_name,
		        COALESCE(t.color, '') as tag_color,
		        COALESCE(n.text, '') as note_text
		 FROM focus_events fe
		 LEFT JOIN event_tags et ON et.event_id = fe.id
		 LEFT JOIN tags t ON t.id = et.tag_id
		 LEFT JOIN notes n ON n.anchor_event = fe.id
		 WHERE fe.started_at >= ? AND fe.started_at <= ?
		 ORDER BY fe.started_at DESC`, dayStart, dayEnd,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, "failed to query events: "+err.Error())
		return
	}
	defer rows.Close()

	type EventResponse struct {
		ID          int64  `json:"id"`
		AppName     string `json:"app_name"`
		WindowTitle string `json:"window_title"`
		StartedAt   string `json:"started_at"`
		EndedAt     string `json:"ended_at,omitempty"`
		DurationS   *int   `json:"duration_s,omitempty"`
		IsIdle      bool   `json:"is_idle"`
		TagName     string `json:"tag_name,omitempty"`
		TagColor    string `json:"tag_color,omitempty"`
		NoteText    string `json:"note_text,omitempty"`
	}

	var events []EventResponse
	for rows.Next() {
		var e EventResponse
		var endedAt sql.NullString
		var durationS sql.NullInt64
		var isIdle int

		if err := rows.Scan(&e.ID, &e.AppName, &e.WindowTitle, &e.StartedAt,
			&endedAt, &durationS, &isIdle, &e.TagName, &e.TagColor, &e.NoteText); err != nil {
			a.jsonError(w, http.StatusInternalServerError, "scan error: "+err.Error())
			return
		}
		if endedAt.Valid {
			e.EndedAt = endedAt.String
		}
		if durationS.Valid {
			d := int(durationS.Int64)
			e.DurationS = &d
		}
		e.IsIdle = isIdle == 1
		events = append(events, e)
	}

	if events == nil {
		events = []EventResponse{}
	}
	a.jsonOK(w, events)
}

// --- Tags ---

func (a *API) handleGetTags(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id, name, color, created_at FROM tags ORDER BY name`)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type TagResponse struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Color     string `json:"color"`
		CreatedAt string `json:"created_at"`
	}

	var tags []TagResponse
	for rows.Next() {
		var t TagResponse
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []TagResponse{}
	}
	a.jsonOK(w, tags)
}

func (a *API) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.Color == "" {
		a.jsonError(w, http.StatusBadRequest, "name and color are required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(`INSERT INTO tags (name, color, created_at) VALUES (?, ?, ?)`,
		req.Name, req.Color, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			a.jsonError(w, http.StatusConflict, "tag name already exists")
			return
		}
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	a.jsonOK(w, map[string]any{"id": id, "name": req.Name, "color": req.Color})
}

// --- Tag/Note Event ---

func (a *API) handleTagEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var req struct {
		TagID int64 `json:"tag_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err = a.db.Exec(
		`INSERT OR REPLACE INTO event_tags (event_id, tag_id, source) VALUES (?, ?, 'manual')`,
		eventID, req.TagID,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonOK(w, map[string]string{"status": "ok"})
}

func (a *API) handleAddNote(w http.ResponseWriter, r *http.Request) {
	eventID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid event id")
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Text == "" {
		a.jsonError(w, http.StatusBadRequest, "text is required")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO notes (anchor_event, text, created_at) VALUES (?, ?, ?)`,
		eventID, req.Text, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	a.jsonOK(w, map[string]any{"id": id})
}

// --- Submissions ---

func (a *API) handleGetSubmissions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(
		`SELECT id, server_id, submitted_at, status, retry_count
		 FROM submissions ORDER BY submitted_at DESC LIMIT 50`,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type SubResponse struct {
		ID          int64  `json:"id"`
		ServerID    string `json:"server_id,omitempty"`
		SubmittedAt string `json:"submitted_at"`
		Status      string `json:"status"`
		RetryCount  int    `json:"retry_count"`
	}

	var subs []SubResponse
	for rows.Next() {
		var s SubResponse
		var serverID sql.NullString
		if err := rows.Scan(&s.ID, &serverID, &s.SubmittedAt, &s.Status, &s.RetryCount); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if serverID.Valid {
			s.ServerID = serverID.String
		}
		subs = append(subs, s)
	}
	if subs == nil {
		subs = []SubResponse{}
	}
	a.jsonOK(w, subs)
}

func (a *API) handleSubmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventIDs []int64 `json:"event_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.EventIDs) == 0 {
		a.jsonError(w, http.StatusBadRequest, "event_ids required")
		return
	}

	// Create submission via SubmitService (injected at runtime; here we do it inline)
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := a.db.Begin()
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO submissions (submitted_at, status, retry_count) VALUES (?, 'pending', 0)`, now)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	subID, _ := result.LastInsertId()

	for _, eid := range req.EventIDs {
		if _, err := tx.Exec(`INSERT INTO submission_events (submission_id, event_id) VALUES (?, ?)`,
			subID, eid); err != nil {
			a.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("link event %d: %s", eid, err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.jsonOK(w, map[string]any{"submission_id": subID, "status": "pending"})
}

// --- Pomodoro ---

func (a *API) handleGetPomodoro(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(
		`SELECT id, started_at, ended_at, work_mins, break_mins, status, tag_id
		 FROM pomodoro_sessions ORDER BY started_at DESC LIMIT 20`,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type PomoResponse struct {
		ID        int64  `json:"id"`
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at,omitempty"`
		WorkMins  int    `json:"work_mins"`
		BreakMins int    `json:"break_mins"`
		Status    string `json:"status"`
		TagID     *int64 `json:"tag_id,omitempty"`
	}

	var sessions []PomoResponse
	for rows.Next() {
		var p PomoResponse
		var endedAt sql.NullString
		var tagID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.StartedAt, &endedAt, &p.WorkMins, &p.BreakMins,
			&p.Status, &tagID); err != nil {
			a.jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if endedAt.Valid {
			p.EndedAt = endedAt.String
		}
		if tagID.Valid {
			v := tagID.Int64
			p.TagID = &v
		}
		sessions = append(sessions, p)
	}
	if sessions == nil {
		sessions = []PomoResponse{}
	}
	a.jsonOK(w, sessions)
}

func (a *API) handleStartPomodoro(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkMins  int    `json:"work_mins"`
		BreakMins int    `json:"break_mins"`
		TagID     *int64 `json:"tag_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.WorkMins <= 0 {
		req.WorkMins = 25
	}
	if req.BreakMins <= 0 {
		req.BreakMins = 5
	}

	now := time.Now().UTC().Format(time.RFC3339)
	result, err := a.db.Exec(
		`INSERT INTO pomodoro_sessions (started_at, work_mins, break_mins, status, tag_id, created_at)
		 VALUES (?, ?, ?, 'work', ?, ?)`,
		now, req.WorkMins, req.BreakMins, req.TagID, now,
	)
	if err != nil {
		a.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, _ := result.LastInsertId()
	a.jsonOK(w, map[string]any{"id": id, "status": "work"})
}

// --- Config ---

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	row := a.db.QueryRow(
		`SELECT device_id, server_url, tracking_on, autostart, presence_intervals, pomodoro_defaults
		 FROM config WHERE id = 1`,
	)

	type ConfigResponse struct {
		DeviceID          string `json:"device_id"`
		ServerURL         string `json:"server_url"`
		TrackingOn        bool   `json:"tracking_on"`
		Autostart         bool   `json:"autostart"`
		PresenceIntervals string `json:"presence_intervals"`
		PomodoroDefaults  string `json:"pomodoro_defaults"`
	}

	var c ConfigResponse
	var trackingOn, autostart int
	if err := row.Scan(&c.DeviceID, &c.ServerURL, &trackingOn, &autostart,
		&c.PresenceIntervals, &c.PomodoroDefaults); err != nil {
		a.jsonError(w, http.StatusInternalServerError, "config not found: "+err.Error())
		return
	}
	c.TrackingOn = trackingOn == 1
	c.Autostart = autostart == 1

	a.jsonOK(w, c)
}

func (a *API) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.jsonError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Supported fields for update
	allowed := map[string]string{
		"tracking_on":        "tracking_on",
		"autostart":          "autostart",
		"presence_intervals": "presence_intervals",
		"pomodoro_defaults":  "pomodoro_defaults",
	}

	for key, val := range req {
		col, ok := allowed[key]
		if !ok {
			continue
		}
		if _, err := a.db.Exec(
			fmt.Sprintf(`UPDATE config SET %s = ? WHERE id = 1`, col), val,
		); err != nil {
			a.jsonError(w, http.StatusInternalServerError, fmt.Sprintf("update %s: %s", key, err))
			return
		}
	}

	a.jsonOK(w, map[string]string{"status": "ok"})
}

// --- Helpers ---

func (a *API) jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(data)
}

func (a *API) jsonError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
```

- [ ] **16.2** Wire API into Server

Update `internal/client/webui/server.go` — add to `registerRoutes()`:

```go
// In registerRoutes(), after the static file handler:
api := NewAPI(s.db, s.logger)
api.RegisterRoutes(s.mux)
```

- [ ] **16.3** Write API tests

```go
// internal/client/webui/api_test.go
package webui

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func setupAPIDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE focus_events (id INTEGER PRIMARY KEY, app_name TEXT NOT NULL,
			window_title TEXT NOT NULL, started_at TEXT NOT NULL, ended_at TEXT,
			duration_s INTEGER, is_idle INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE,
			color TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE event_tags (event_id INTEGER, tag_id INTEGER, source TEXT,
			cascade_from INTEGER, UNIQUE(event_id, tag_id))`,
		`CREATE TABLE notes (id INTEGER PRIMARY KEY, anchor_event INTEGER, text TEXT,
			created_at TEXT, cascade_applied INTEGER DEFAULT 0)`,
		`CREATE TABLE submissions (id INTEGER PRIMARY KEY, server_id TEXT,
			submitted_at TEXT NOT NULL, status TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0, last_retry TEXT)`,
		`CREATE TABLE submission_events (submission_id INTEGER, event_id INTEGER,
			UNIQUE(submission_id, event_id))`,
		`CREATE TABLE pomodoro_sessions (id INTEGER PRIMARY KEY, started_at TEXT NOT NULL,
			ended_at TEXT, work_mins INTEGER DEFAULT 25, break_mins INTEGER DEFAULT 5,
			status TEXT NOT NULL, tag_id INTEGER, created_at TEXT NOT NULL)`,
		`CREATE TABLE config (id INTEGER PRIMARY KEY CHECK (id = 1), device_id TEXT NOT NULL,
			server_url TEXT NOT NULL, api_key TEXT NOT NULL,
			tracking_on INTEGER NOT NULL DEFAULT 1, autostart INTEGER NOT NULL DEFAULT 0,
			presence_intervals TEXT NOT NULL DEFAULT '[30,45,60,90,120]',
			pomodoro_defaults TEXT NOT NULL DEFAULT '{"work":25,"break":5}')`,
		// Seed
		`INSERT INTO config (id, device_id, server_url, api_key) VALUES (1, 'test-dev', 'https://test.example.com', 'key-123')`,
		`INSERT INTO tags (id, name, color, created_at) VALUES (1, 'Dev', '#00ff00', '2026-01-01T00:00:00Z')`,
		`INSERT INTO focus_events (id, app_name, window_title, started_at, ended_at, duration_s, created_at)
		 VALUES (1, 'terminal', 'claude', '2026-03-23T09:00:00Z', '2026-03-23T09:30:00Z', 1800, '2026-03-23T09:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

func startTestServer(t *testing.T, db *sql.DB) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv := NewServer(db, 0, logger)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })
	return srv
}

func TestAPI_GetEvents(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/events?date=2026-03-23")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var events []map[string]any
	json.NewDecoder(resp.Body).Decode(&events)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestAPI_GetTags(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/tags")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var tags []map[string]any
	json.NewDecoder(resp.Body).Decode(&tags)
	if len(tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(tags))
	}
}

func TestAPI_CreateTag(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"name":"Testing","color":"#ff0000"}`)
	resp, err := http.Post(srv.URL()+"/api/tags", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["name"] != "Testing" {
		t.Errorf("expected name 'Testing', got %v", result["name"])
	}
}

func TestAPI_TagEvent(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"tag_id":1}`)
	resp, err := http.Post(srv.URL()+"/api/events/1/tag", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAPI_AddNote(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"text":"Working on auth module"}`)
	resp, err := http.Post(srv.URL()+"/api/events/1/note", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAPI_GetConfig(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	resp, err := http.Get(srv.URL() + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var config map[string]any
	json.NewDecoder(resp.Body).Decode(&config)
	if config["device_id"] != "test-dev" {
		t.Errorf("expected device_id 'test-dev', got %v", config["device_id"])
	}
}

func TestAPI_UpdateConfig(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"tracking_on":0}`)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL()+"/api/config", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the change
	var trackingOn int
	db.QueryRow(`SELECT tracking_on FROM config WHERE id = 1`).Scan(&trackingOn)
	if trackingOn != 0 {
		t.Errorf("expected tracking_on=0, got %d", trackingOn)
	}
}

func TestAPI_Submit(t *testing.T) {
	db := setupAPIDB(t)
	srv := startTestServer(t, db)

	body := bytes.NewBufferString(`{"event_ids":[1]}`)
	resp, err := http.Post(srv.URL()+"/api/submit", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", result["status"])
	}
}
```

- [ ] **16.4** Run tests

```bash
go test ./internal/client/webui/ -v -run TestAPI
# Expected: all tests PASS
```

- [ ] **16.5** Commit

```bash
git add internal/client/webui/api.go internal/client/webui/api_test.go
git commit -m "feat(webui): add REST API endpoints for events, tags, notes, submissions, pomodoro, config"
```

---

## Task 17: Local Web UI — Dashboard Page

**Files:**
- Create: `web/client-ui/src/routes/+page.svelte`
- Create: `web/client-ui/src/lib/api.ts`
- Create: `web/client-ui/src/lib/types.ts`

### Steps

- [ ] **17.1** Create shared types

```ts
// web/client-ui/src/lib/types.ts
export interface FocusEvent {
  id: number;
  app_name: string;
  window_title: string;
  started_at: string;
  ended_at?: string;
  duration_s?: number;
  is_idle: boolean;
  tag_name?: string;
  tag_color?: string;
  note_text?: string;
}

export interface Tag {
  id: number;
  name: string;
  color: string;
  created_at: string;
}

export interface Submission {
  id: number;
  server_id?: string;
  submitted_at: string;
  status: string;
  retry_count: number;
}

export interface PomodoroSession {
  id: number;
  started_at: string;
  ended_at?: string;
  work_mins: number;
  break_mins: number;
  status: string;
  tag_id?: number;
}

export interface Config {
  device_id: string;
  server_url: string;
  tracking_on: boolean;
  autostart: boolean;
  presence_intervals: string;
  pomodoro_defaults: string;
}

export interface TagSummary {
  tag: string;
  color: string;
  totalSeconds: number;
  percentage: number;
}
```

- [ ] **17.2** Create API client

```ts
// web/client-ui/src/lib/api.ts
const BASE = '';  // Same origin — Go serves both SPA and API

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json();
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`POST ${path}: ${res.status}`);
  return res.json();
}

async function patch<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`PATCH ${path}: ${res.status}`);
  return res.json();
}

import type { FocusEvent, Tag, Submission, PomodoroSession, Config } from './types';

export const api = {
  getEvents: (date?: string) =>
    get<FocusEvent[]>(`/api/events${date ? `?date=${date}` : ''}`),

  getTags: () => get<Tag[]>('/api/tags'),

  createTag: (name: string, color: string) =>
    post<Tag>('/api/tags', { name, color }),

  tagEvent: (eventId: number, tagId: number) =>
    post<{ status: string }>(`/api/events/${eventId}/tag`, { tag_id: tagId }),

  addNote: (eventId: number, text: string) =>
    post<{ id: number }>(`/api/events/${eventId}/note`, { text }),

  getSubmissions: () => get<Submission[]>('/api/submissions'),

  submit: (eventIds: number[]) =>
    post<{ submission_id: number; status: string }>('/api/submit', { event_ids: eventIds }),

  getPomodoro: () => get<PomodoroSession[]>('/api/pomodoro'),

  startPomodoro: (workMins: number, breakMins: number, tagId?: number) =>
    post<{ id: number; status: string }>('/api/pomodoro/start', {
      work_mins: workMins,
      break_mins: breakMins,
      tag_id: tagId,
    }),

  getConfig: () => get<Config>('/api/config'),

  updateConfig: (updates: Partial<Config>) =>
    patch<{ status: string }>('/api/config', updates),
};
```

- [ ] **17.3** Create the dashboard page

```svelte
<!-- web/client-ui/src/routes/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent, TagSummary } from '$lib/types';

  let events: FocusEvent[] = [];
  let tagSummaries: TagSummary[] = [];
  let currentFocus: FocusEvent | null = null;
  let totalTrackedSeconds = 0;
  let loading = true;

  onMount(async () => {
    try {
      events = await api.getEvents();
      computeSummaries();
      if (events.length > 0) {
        currentFocus = events[0]; // Most recent
      }
    } catch (e) {
      console.error('Failed to load events:', e);
    } finally {
      loading = false;
    }
  });

  function computeSummaries() {
    const byTag = new Map<string, { color: string; seconds: number }>();
    let total = 0;

    for (const event of events) {
      if (event.is_idle) continue;
      const seconds = event.duration_s ?? 0;
      total += seconds;
      const tag = event.tag_name || 'Untagged';
      const color = event.tag_color || '#999999';
      const existing = byTag.get(tag) ?? { color, seconds: 0 };
      existing.seconds += seconds;
      byTag.set(tag, existing);
    }

    totalTrackedSeconds = total;
    tagSummaries = Array.from(byTag.entries())
      .map(([tag, { color, seconds }]) => ({
        tag,
        color,
        totalSeconds: seconds,
        percentage: total > 0 ? Math.round((seconds / total) * 100) : 0,
      }))
      .sort((a, b) => b.totalSeconds - a.totalSeconds);
  }

  function formatDuration(seconds: number): string {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return h > 0 ? `${h}h ${m}m` : `${m}m`;
  }
</script>

<svelte:head>
  <title>Trasker — Dashboard</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Today's Activity</h1>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Current Focus -->
    {#if currentFocus}
      <div class="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
        <p class="text-sm text-blue-600 font-medium">Currently Focused On</p>
        <p class="text-lg font-semibold">{currentFocus.app_name}</p>
        <p class="text-gray-600 text-sm truncate">{currentFocus.window_title}</p>
        {#if currentFocus.tag_name}
          <span class="inline-block mt-1 px-2 py-0.5 rounded text-xs text-white"
                style="background-color: {currentFocus.tag_color}">
            {currentFocus.tag_name}
          </span>
        {/if}
      </div>
    {/if}

    <!-- Total Time -->
    <div class="mb-6">
      <p class="text-gray-500 text-sm">Total tracked today</p>
      <p class="text-3xl font-bold">{formatDuration(totalTrackedSeconds)}</p>
    </div>

    <!-- Tag Breakdown -->
    <div class="space-y-3">
      <h2 class="text-lg font-semibold">By Tag</h2>
      {#each tagSummaries as summary}
        <div class="flex items-center gap-3">
          <div class="w-3 h-3 rounded-full" style="background-color: {summary.color}"></div>
          <span class="flex-1 font-medium">{summary.tag}</span>
          <span class="text-gray-600">{formatDuration(summary.totalSeconds)}</span>
          <div class="w-24 bg-gray-200 rounded-full h-2">
            <div class="h-2 rounded-full" style="width: {summary.percentage}%; background-color: {summary.color}"></div>
          </div>
          <span class="text-sm text-gray-500 w-10 text-right">{summary.percentage}%</span>
        </div>
      {/each}
      {#if tagSummaries.length === 0}
        <p class="text-gray-400">No activity tracked yet today.</p>
      {/if}
    </div>
  {/if}
</div>
```

- [ ] **17.4** Verify build

```bash
cd web/client-ui && npm run build
# Expected: Build succeeds
```

- [ ] **17.5** Commit

```bash
git add web/client-ui/src/lib/types.ts web/client-ui/src/lib/api.ts web/client-ui/src/routes/+page.svelte
git commit -m "feat(webui): add dashboard page with tag summary, current focus, and time chart"
```

---

## Task 18: Local Web UI — Timeline Page

**Files:**
- Create: `web/client-ui/src/routes/timeline/+page.svelte`

### Steps

- [ ] **18.1** Create the timeline page

```svelte
<!-- web/client-ui/src/routes/timeline/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent, Tag } from '$lib/types';

  let events: FocusEvent[] = [];
  let tags: Tag[] = [];
  let loading = true;
  let editingNote: number | null = null;
  let noteText = '';

  onMount(async () => {
    try {
      [events, tags] = await Promise.all([api.getEvents(), api.getTags()]);
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  async function setTag(eventId: number, tagId: number) {
    try {
      await api.tagEvent(eventId, tagId);
      events = await api.getEvents();
    } catch (e) {
      console.error('Failed to tag:', e);
    }
  }

  async function saveNote(eventId: number) {
    if (!noteText.trim()) return;
    try {
      await api.addNote(eventId, noteText.trim());
      events = await api.getEvents();
      editingNote = null;
      noteText = '';
    } catch (e) {
      console.error('Failed to save note:', e);
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(seconds: number | undefined): string {
    if (!seconds) return '-';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m > 0 ? `${m}m ${s}s` : `${s}s`;
  }
</script>

<svelte:head>
  <title>Trasker — Timeline</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Timeline</h1>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <div class="space-y-2">
      {#each events as event}
        <div class="border rounded-lg p-3 hover:bg-gray-50 {event.is_idle ? 'opacity-50' : ''}">
          <div class="flex items-center gap-3">
            <!-- Time -->
            <span class="text-sm text-gray-500 w-16 shrink-0">
              {formatTime(event.started_at)}
            </span>

            <!-- App + Title -->
            <div class="flex-1 min-w-0">
              <p class="font-medium truncate">{event.app_name}</p>
              <p class="text-sm text-gray-600 truncate">{event.window_title}</p>
            </div>

            <!-- Duration -->
            <span class="text-sm text-gray-500 w-16 text-right">
              {formatDuration(event.duration_s)}
            </span>

            <!-- Tag -->
            <div class="w-32">
              {#if event.tag_name}
                <span class="px-2 py-0.5 rounded text-xs text-white"
                      style="background-color: {event.tag_color}">
                  {event.tag_name}
                </span>
              {:else}
                <select class="text-xs border rounded p-1 w-full"
                        on:change={(e) => setTag(event.id, parseInt(e.currentTarget.value))}>
                  <option value="">Tag...</option>
                  {#each tags as tag}
                    <option value={tag.id}>{tag.name}</option>
                  {/each}
                </select>
              {/if}
            </div>

            <!-- Note button -->
            <button class="text-gray-400 hover:text-blue-500 text-sm"
                    on:click={() => { editingNote = editingNote === event.id ? null : event.id; noteText = event.note_text ?? ''; }}>
              {event.note_text ? '📝' : '+note'}
            </button>
          </div>

          <!-- Note editor -->
          {#if editingNote === event.id}
            <div class="mt-2 flex gap-2">
              <input type="text" bind:value={noteText}
                     class="flex-1 border rounded px-2 py-1 text-sm"
                     placeholder="Add a note..."
                     on:keydown={(e) => e.key === 'Enter' && saveNote(event.id)} />
              <button class="px-3 py-1 bg-blue-500 text-white rounded text-sm hover:bg-blue-600"
                      on:click={() => saveNote(event.id)}>
                Save
              </button>
            </div>
          {/if}

          <!-- Existing note display -->
          {#if event.note_text && editingNote !== event.id}
            <p class="mt-1 text-sm text-gray-600 italic ml-19">{event.note_text}</p>
          {/if}
        </div>
      {/each}

      {#if events.length === 0}
        <p class="text-gray-400 text-center py-8">No events tracked today.</p>
      {/if}
    </div>
  {/if}
</div>
```

- [ ] **18.2** Verify build

```bash
cd web/client-ui && npm run build
# Expected: Build succeeds
```

- [ ] **18.3** Commit

```bash
git add web/client-ui/src/routes/timeline/+page.svelte
git commit -m "feat(webui): add timeline page with inline tag/note editing"
```

---

## Task 19: Local Web UI — Submit Page

**Files:**
- Create: `web/client-ui/src/routes/submit/+page.svelte`

### Steps

- [ ] **19.1** Create the submit page

```svelte
<!-- web/client-ui/src/routes/submit/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { FocusEvent } from '$lib/types';

  let events: FocusEvent[] = [];
  let selected = new Set<number>();
  let loading = true;
  let submitting = false;
  let showConfirm = false;
  let submitResult: string | null = null;

  onMount(async () => {
    try {
      events = await api.getEvents();
      // Filter to only tagged, non-idle events
      events = events.filter(e => !e.is_idle && e.tag_name);
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  function toggleSelect(id: number) {
    if (selected.has(id)) {
      selected.delete(id);
    } else {
      selected.add(id);
    }
    selected = new Set(selected); // trigger reactivity
  }

  function selectAll() {
    if (selected.size === events.length) {
      selected = new Set();
    } else {
      selected = new Set(events.map(e => e.id));
    }
  }

  async function confirmSubmit() {
    submitting = true;
    try {
      const result = await api.submit(Array.from(selected));
      submitResult = `Submitted successfully! Submission ID: ${result.submission_id}`;
      selected = new Set();
      showConfirm = false;
    } catch (e) {
      submitResult = `Submission failed: ${e}`;
    } finally {
      submitting = false;
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(seconds: number | undefined): string {
    if (!seconds) return '-';
    const m = Math.floor(seconds / 60);
    return `${m}m`;
  }
</script>

<svelte:head>
  <title>Trasker — Submit</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Submit Entries</h1>

  {#if submitResult}
    <div class="mb-4 p-3 rounded {submitResult.includes('failed') ? 'bg-red-50 text-red-700' : 'bg-green-50 text-green-700'}">
      {submitResult}
    </div>
  {/if}

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Actions -->
    <div class="flex items-center gap-4 mb-4">
      <button class="text-sm text-blue-500 hover:underline" on:click={selectAll}>
        {selected.size === events.length ? 'Deselect All' : 'Select All'}
      </button>
      <span class="text-sm text-gray-500">{selected.size} selected</span>
      <button class="ml-auto px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
              disabled={selected.size === 0}
              on:click={() => showConfirm = true}>
        Submit Selected
      </button>
    </div>

    <!-- Event list -->
    <div class="space-y-1">
      {#each events as event}
        <div class="flex items-center gap-3 p-2 rounded hover:bg-gray-50 cursor-pointer"
             on:click={() => toggleSelect(event.id)}
             on:keydown={(e) => e.key === 'Enter' && toggleSelect(event.id)}
             role="checkbox"
             aria-checked={selected.has(event.id)}
             tabindex="0">
          <input type="checkbox" checked={selected.has(event.id)}
                 on:click|stopPropagation={() => toggleSelect(event.id)}
                 class="rounded" />
          <span class="text-sm text-gray-500 w-16">{formatTime(event.started_at)}</span>
          <span class="flex-1 truncate">{event.app_name}</span>
          <span class="px-2 py-0.5 rounded text-xs text-white"
                style="background-color: {event.tag_color}">
            {event.tag_name}
          </span>
          <span class="text-sm text-gray-500 w-12 text-right">{formatDuration(event.duration_s)}</span>
        </div>
      {/each}
    </div>

    {#if events.length === 0}
      <p class="text-gray-400 text-center py-8">No tagged entries to submit. Tag your events first.</p>
    {/if}
  {/if}

  <!-- Confirmation dialog -->
  {#if showConfirm}
    <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg p-6 max-w-md mx-4">
        <h2 class="text-lg font-bold mb-3">Confirm Submission</h2>
        <p class="text-gray-600 mb-4">
          Submitted entries cannot be edited or deleted from your client.
          Only your org admin can modify or remove submitted entries.
          Are you sure?
        </p>
        <p class="text-sm text-gray-500 mb-4">{selected.size} entries will be submitted.</p>
        <div class="flex gap-3 justify-end">
          <button class="px-4 py-2 border rounded hover:bg-gray-50"
                  on:click={() => showConfirm = false}>
            Cancel
          </button>
          <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 disabled:opacity-50"
                  disabled={submitting}
                  on:click={confirmSubmit}>
            {submitting ? 'Submitting...' : 'Submit'}
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>
```

- [ ] **19.2** Verify build

```bash
cd web/client-ui && npm run build
# Expected: Build succeeds
```

- [ ] **19.3** Commit

```bash
git add web/client-ui/src/routes/submit/+page.svelte
git commit -m "feat(webui): add submit page with batch selection and confirmation dialog"
```

---

## Task 20: Local Web UI — Pomodoro Page

**Files:**
- Create: `web/client-ui/src/routes/pomodoro/+page.svelte`

### Steps

- [ ] **20.1** Create the pomodoro page

```svelte
<!-- web/client-ui/src/routes/pomodoro/+page.svelte -->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import type { PomodoroSession, Tag } from '$lib/types';

  let sessions: PomodoroSession[] = [];
  let tags: Tag[] = [];
  let loading = true;
  let workMins = 25;
  let breakMins = 5;
  let selectedTagId: number | undefined;

  // Live timer state (polled from API or computed locally)
  let activePomo: PomodoroSession | null = null;
  let timerDisplay = '00:00';
  let timerInterval: ReturnType<typeof setInterval> | null = null;

  onMount(async () => {
    try {
      [sessions, tags] = await Promise.all([api.getPomodoro(), api.getTags()]);
      activePomo = sessions.find(s => s.status === 'work' || s.status === 'break') ?? null;
      if (activePomo) startTimerDisplay();
    } catch (e) {
      console.error('Failed to load:', e);
    } finally {
      loading = false;
    }
  });

  onDestroy(() => {
    if (timerInterval) clearInterval(timerInterval);
  });

  async function startPomodoro() {
    try {
      await api.startPomodoro(workMins, breakMins, selectedTagId);
      sessions = await api.getPomodoro();
      activePomo = sessions.find(s => s.status === 'work' || s.status === 'break') ?? null;
      if (activePomo) startTimerDisplay();
    } catch (e) {
      console.error('Failed to start:', e);
    }
  }

  function startTimerDisplay() {
    if (timerInterval) clearInterval(timerInterval);
    timerInterval = setInterval(updateTimer, 1000);
    updateTimer();
  }

  function updateTimer() {
    if (!activePomo) {
      timerDisplay = '00:00';
      return;
    }
    const started = new Date(activePomo.started_at).getTime();
    const durationMs = (activePomo.status === 'work' ? activePomo.work_mins : activePomo.break_mins) * 60 * 1000;
    const remaining = Math.max(0, durationMs - (Date.now() - started));
    const mins = Math.floor(remaining / 60000);
    const secs = Math.floor((remaining % 60000) / 1000);
    timerDisplay = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`;

    if (remaining === 0) {
      // Refresh from server
      api.getPomodoro().then(s => {
        sessions = s;
        activePomo = s.find(sess => sess.status === 'work' || sess.status === 'break') ?? null;
        if (!activePomo && timerInterval) {
          clearInterval(timerInterval);
          timerInterval = null;
        }
      });
    }
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  $: completedCount = sessions.filter(s => s.status === 'done').length;
  $: totalWorkMins = sessions.filter(s => s.status === 'done').reduce((sum, s) => sum + s.work_mins, 0);
</script>

<svelte:head>
  <title>Trasker — Pomodoro</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Pomodoro Timer</h1>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Timer display -->
    <div class="text-center mb-8">
      <p class="text-6xl font-mono font-bold mb-2">{timerDisplay}</p>
      {#if activePomo}
        <p class="text-lg text-gray-600 capitalize">{activePomo.status} phase</p>
      {:else}
        <p class="text-lg text-gray-400">Ready to start</p>
      {/if}
    </div>

    <!-- Controls -->
    {#if !activePomo}
      <div class="flex items-center gap-4 justify-center mb-8">
        <label class="text-sm">
          Work: <input type="number" bind:value={workMins} min="1" max="90"
                       class="w-16 border rounded px-2 py-1" /> min
        </label>
        <label class="text-sm">
          Break: <input type="number" bind:value={breakMins} min="1" max="30"
                        class="w-16 border rounded px-2 py-1" /> min
        </label>
        <select bind:value={selectedTagId} class="border rounded px-2 py-1 text-sm">
          <option value={undefined}>No tag</option>
          {#each tags as tag}
            <option value={tag.id}>{tag.name}</option>
          {/each}
        </select>
        <button class="px-6 py-2 bg-green-500 text-white rounded-lg hover:bg-green-600 font-medium"
                on:click={startPomodoro}>
          Start
        </button>
      </div>
    {/if}

    <!-- Stats -->
    <div class="grid grid-cols-2 gap-4 mb-8">
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <p class="text-3xl font-bold">{completedCount}</p>
        <p class="text-sm text-gray-500">Completed today</p>
      </div>
      <div class="bg-gray-50 rounded-lg p-4 text-center">
        <p class="text-3xl font-bold">{totalWorkMins}m</p>
        <p class="text-sm text-gray-500">Focus time</p>
      </div>
    </div>

    <!-- History -->
    <h2 class="text-lg font-semibold mb-3">Recent Sessions</h2>
    <div class="space-y-2">
      {#each sessions.slice(0, 10) as session}
        <div class="flex items-center gap-3 p-2 border rounded text-sm">
          <span class="text-gray-500">{formatTime(session.started_at)}</span>
          <span class="capitalize font-medium {session.status === 'done' ? 'text-green-600' : session.status === 'cancelled' ? 'text-red-500' : 'text-blue-500'}">
            {session.status}
          </span>
          <span class="text-gray-500">{session.work_mins}m / {session.break_mins}m</span>
        </div>
      {/each}
    </div>
  {/if}
</div>
```

- [ ] **20.2** Verify build and commit

```bash
cd web/client-ui && npm run build
git add web/client-ui/src/routes/pomodoro/+page.svelte
git commit -m "feat(webui): add pomodoro page with timer display, controls, and history"
```

---

## Task 21: Local Web UI — Settings Page

**Files:**
- Create: `web/client-ui/src/routes/settings/+page.svelte`

### Steps

- [ ] **21.1** Create the settings page

```svelte
<!-- web/client-ui/src/routes/settings/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Config } from '$lib/types';

  let config: Config | null = null;
  let loading = true;
  let saving = false;
  let message = '';

  onMount(async () => {
    try {
      config = await api.getConfig();
    } catch (e) {
      console.error('Failed to load config:', e);
    } finally {
      loading = false;
    }
  });

  async function toggleTracking() {
    if (!config) return;
    saving = true;
    try {
      const newVal = !config.tracking_on;
      await api.updateConfig({ tracking_on: newVal } as any);
      config.tracking_on = newVal;
      message = `Tracking ${newVal ? 'enabled' : 'disabled'}`;
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  async function toggleAutostart() {
    if (!config) return;
    saving = true;
    try {
      const newVal = !config.autostart;
      await api.updateConfig({ autostart: newVal } as any);
      config.autostart = newVal;
      message = `Autostart ${newVal ? 'enabled' : 'disabled'}`;
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  async function savePomodoro(workMins: number, breakMins: number) {
    saving = true;
    try {
      const defaults = JSON.stringify({ work: workMins, break: breakMins });
      await api.updateConfig({ pomodoro_defaults: defaults } as any);
      message = 'Pomodoro defaults saved';
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    } finally {
      saving = false;
    }
  }

  $: pomodoroDefaults = config?.pomodoro_defaults ? JSON.parse(config.pomodoro_defaults) : { work: 25, break: 5 };
</script>

<svelte:head>
  <title>Trasker — Settings</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Settings</h1>

  {#if message}
    <div class="mb-4 p-3 bg-blue-50 text-blue-700 rounded">{message}</div>
  {/if}

  {#if loading || !config}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <div class="space-y-6">
      <!-- Device Info -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Device</h2>
        <div class="bg-gray-50 rounded-lg p-4 space-y-1 text-sm">
          <p><span class="text-gray-500">Device ID:</span> {config.device_id}</p>
          <p><span class="text-gray-500">Server:</span> {config.server_url}</p>
        </div>
      </section>

      <!-- Tracking -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Tracking</h2>
        <div class="flex items-center gap-4">
          <button class="px-4 py-2 rounded {config.tracking_on ? 'bg-green-500 text-white' : 'bg-gray-200 text-gray-700'}"
                  disabled={saving}
                  on:click={toggleTracking}>
            {config.tracking_on ? 'Tracking ON' : 'Tracking OFF'}
          </button>
        </div>
      </section>

      <!-- Autostart -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Autostart</h2>
        <div class="flex items-center gap-4">
          <button class="px-4 py-2 rounded {config.autostart ? 'bg-green-500 text-white' : 'bg-gray-200 text-gray-700'}"
                  disabled={saving}
                  on:click={toggleAutostart}>
            {config.autostart ? 'Start with OS: ON' : 'Start with OS: OFF'}
          </button>
        </div>
      </section>

      <!-- Pomodoro Defaults -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Pomodoro Defaults</h2>
        <div class="flex items-center gap-4">
          <label class="text-sm">
            Work: <input type="number" value={pomodoroDefaults.work} min="1" max="90"
                         class="w-16 border rounded px-2 py-1"
                         on:change={(e) => pomodoroDefaults.work = parseInt(e.currentTarget.value)} /> min
          </label>
          <label class="text-sm">
            Break: <input type="number" value={pomodoroDefaults.break} min="1" max="30"
                          class="w-16 border rounded px-2 py-1"
                          on:change={(e) => pomodoroDefaults.break = parseInt(e.currentTarget.value)} /> min
          </label>
          <button class="px-3 py-1 bg-blue-500 text-white rounded text-sm"
                  on:click={() => savePomodoro(pomodoroDefaults.work, pomodoroDefaults.break)}>
            Save
          </button>
        </div>
      </section>

      <!-- Presence Intervals -->
      <section>
        <h2 class="text-lg font-semibold mb-2">Presence Check Intervals</h2>
        <p class="text-sm text-gray-600">{config.presence_intervals}</p>
        <p class="text-xs text-gray-400 mt-1">Minutes before each "Still there?" check when no focus change occurs.</p>
      </section>
    </div>
  {/if}
</div>
```

- [ ] **21.2** Verify build and commit

```bash
cd web/client-ui && npm run build
git add web/client-ui/src/routes/settings/+page.svelte
git commit -m "feat(webui): add settings page with tracking, autostart, and pomodoro config"
```

---

## Task 22: Local Web UI — Tags Page

**Files:**
- Create: `web/client-ui/src/routes/tags/+page.svelte`

### Steps

- [ ] **22.1** Create the tags management page

```svelte
<!-- web/client-ui/src/routes/tags/+page.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import type { Tag } from '$lib/types';

  let tags: Tag[] = [];
  let loading = true;
  let newName = '';
  let newColor = '#3b82f6';
  let message = '';

  // TODO: Wire tag_rules via additional API endpoints when tagger store API is added
  // For now, this page manages tags only. Rule management will be added in a follow-up.

  onMount(async () => {
    try {
      tags = await api.getTags();
    } catch (e) {
      console.error('Failed to load tags:', e);
    } finally {
      loading = false;
    }
  });

  async function createTag() {
    if (!newName.trim()) return;
    try {
      await api.createTag(newName.trim(), newColor);
      tags = await api.getTags();
      newName = '';
      message = 'Tag created';
      setTimeout(() => message = '', 3000);
    } catch (e) {
      message = `Failed: ${e}`;
    }
  }
</script>

<svelte:head>
  <title>Trasker — Tags</title>
</svelte:head>

<div class="max-w-4xl mx-auto p-6">
  <h1 class="text-2xl font-bold mb-6">Tags</h1>

  {#if message}
    <div class="mb-4 p-3 bg-blue-50 text-blue-700 rounded">{message}</div>
  {/if}

  <!-- Create new tag -->
  <div class="flex items-center gap-3 mb-6">
    <input type="text" bind:value={newName}
           class="flex-1 border rounded px-3 py-2"
           placeholder="New tag name..."
           on:keydown={(e) => e.key === 'Enter' && createTag()} />
    <input type="color" bind:value={newColor}
           class="w-10 h-10 rounded cursor-pointer border-0" />
    <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
            on:click={createTag}>
      Create
    </button>
  </div>

  {#if loading}
    <p class="text-gray-500">Loading...</p>
  {:else}
    <!-- Tag list -->
    <div class="space-y-2">
      {#each tags as tag}
        <div class="flex items-center gap-3 p-3 border rounded">
          <div class="w-6 h-6 rounded" style="background-color: {tag.color}"></div>
          <span class="font-medium flex-1">{tag.name}</span>
          <span class="text-xs text-gray-400">Created {new Date(tag.created_at).toLocaleDateString()}</span>
        </div>
      {/each}

      {#if tags.length === 0}
        <p class="text-gray-400 text-center py-8">No tags yet. Create your first tag above.</p>
      {/if}
    </div>

    <!-- Auto-tag rules section (placeholder for tagger integration) -->
    <h2 class="text-lg font-semibold mt-8 mb-3">Auto-Tag Rules</h2>
    <p class="text-gray-400 text-sm">Auto-tag rule management will be available in a future update. Rules are currently managed through the tagger engine.</p>
  {/if}
</div>
```

- [ ] **22.2** Add navigation layout

```svelte
<!-- web/client-ui/src/routes/+layout.svelte -->
<script lang="ts">
  import '../app.css';
  import { page } from '$app/stores';

  const navItems = [
    { href: '/', label: 'Dashboard' },
    { href: '/timeline', label: 'Timeline' },
    { href: '/submit', label: 'Submit' },
    { href: '/pomodoro', label: 'Pomodoro' },
    { href: '/tags', label: 'Tags' },
    { href: '/settings', label: 'Settings' },
  ];
</script>

<div class="min-h-screen bg-white">
  <nav class="border-b">
    <div class="max-w-4xl mx-auto px-6 flex items-center gap-6 h-14">
      <span class="font-bold text-lg">Trasker</span>
      {#each navItems as item}
        <a href={item.href}
           class="text-sm hover:text-blue-500 {$page.url.pathname === item.href ? 'text-blue-500 font-medium' : 'text-gray-600'}">
          {item.label}
        </a>
      {/each}
    </div>
  </nav>

  <slot />
</div>
```

- [ ] **22.3** Verify build and commit

```bash
cd web/client-ui && npm run build
git add web/client-ui/src/routes/tags/+page.svelte web/client-ui/src/routes/+layout.svelte
git commit -m "feat(webui): add tags page and navigation layout for all dashboard pages"
```

---

## Task 23: First Run Flow

**Files:**
- Create: `internal/client/setup/firstrun.go`
- Create: `internal/client/setup/firstrun_test.go`

### Steps

- [ ] **23.1** Create the first-run setup logic

```go
// internal/client/setup/firstrun.go
package setup

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
)

// Config holds the baked-in values from build-time ldflags.
type Config struct {
	ServerURL string
	APIKey    string
}

// FirstRun handles initial setup: data dir, SQLite init, device registration.
type FirstRun struct {
	config  Config
	logger  *slog.Logger
	dataDir string
}

// NewFirstRun creates the first-run handler.
func NewFirstRun(config Config, logger *slog.Logger) *FirstRun {
	return &FirstRun{
		config: config,
		logger: logger,
	}
}

// DataDir returns the platform-appropriate data directory.
func DataDir() (string, error) {
	switch runtime.GOOS {
	case "linux":
		// XDG_DATA_HOME or ~/.local/share
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("firstrun: home dir: %w", err)
			}
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, "trasker"), nil

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("firstrun: home dir: %w", err)
		}
		return filepath.Join(home, "Library", "Application Support", "Trasker"), nil

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("firstrun: APPDATA not set")
		}
		return filepath.Join(appData, "Trasker"), nil

	default:
		return "", fmt.Errorf("firstrun: unsupported OS: %s", runtime.GOOS)
	}
}

// DBPath returns the path to the SQLite database file.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trasker.db"), nil
}

// IsFirstRun checks if the data directory and DB exist.
func IsFirstRun() (bool, error) {
	dbPath, err := DBPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(dbPath)
	return os.IsNotExist(err), nil
}

// Run performs the first-run setup. Returns the database connection.
func (f *FirstRun) Run(ctx context.Context) (*sql.DB, error) {
	// 1. Create data directory
	dataDir, err := DataDir()
	if err != nil {
		return nil, err
	}
	f.dataDir = dataDir

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("firstrun: create data dir: %w", err)
	}
	f.logger.Info("data directory created", "path", dataDir)

	// 2. Open SQLite database
	dbPath := filepath.Join(dataDir, "trasker.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("firstrun: open db: %w", err)
	}

	// 3. Initialize schema
	if err := f.initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("firstrun: init schema: %w", err)
	}
	f.logger.Info("database initialized", "path", dbPath)

	// 4. Generate device ID and insert config
	deviceID := uuid.New().String()
	_, err = db.Exec(
		`INSERT OR IGNORE INTO config (id, device_id, server_url, api_key)
		 VALUES (1, ?, ?, ?)`,
		deviceID, f.config.ServerURL, f.config.APIKey,
	)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("firstrun: insert config: %w", err)
	}
	f.logger.Info("device registered locally", "device_id", deviceID)

	return db, nil
}

// initSchema creates all SQLite tables.
func (f *FirstRun) initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS focus_events (
		id           INTEGER PRIMARY KEY,
		app_name     TEXT NOT NULL,
		window_title TEXT NOT NULL,
		started_at   TEXT NOT NULL,
		ended_at     TEXT,
		duration_s   INTEGER,
		is_idle      INTEGER NOT NULL DEFAULT 0,
		created_at   TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tags (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE,
		color      TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS event_tags (
		event_id     INTEGER NOT NULL REFERENCES focus_events(id),
		tag_id       INTEGER NOT NULL REFERENCES tags(id),
		source       TEXT NOT NULL,
		cascade_from INTEGER,
		UNIQUE(event_id, tag_id)
	);

	CREATE TABLE IF NOT EXISTS notes (
		id              INTEGER PRIMARY KEY,
		anchor_event    INTEGER NOT NULL REFERENCES focus_events(id),
		text            TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		cascade_applied INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS tag_rules (
		id            INTEGER PRIMARY KEY,
		tag_id        INTEGER NOT NULL REFERENCES tags(id),
		app_pattern   TEXT NOT NULL,
		title_pattern TEXT,
		priority      INTEGER NOT NULL DEFAULT 0,
		suggested     INTEGER NOT NULL DEFAULT 0,
		hit_count     INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS submissions (
		id           INTEGER PRIMARY KEY,
		server_id    TEXT,
		submitted_at TEXT NOT NULL,
		status       TEXT NOT NULL,
		retry_count  INTEGER NOT NULL DEFAULT 0,
		last_retry   TEXT
	);

	CREATE TABLE IF NOT EXISTS submission_events (
		submission_id INTEGER NOT NULL REFERENCES submissions(id),
		event_id      INTEGER NOT NULL REFERENCES focus_events(id),
		UNIQUE(submission_id, event_id)
	);

	CREATE TABLE IF NOT EXISTS pomodoro_sessions (
		id         INTEGER PRIMARY KEY,
		started_at TEXT NOT NULL,
		ended_at   TEXT,
		work_mins  INTEGER NOT NULL DEFAULT 25,
		break_mins INTEGER NOT NULL DEFAULT 5,
		status     TEXT NOT NULL,
		tag_id     INTEGER REFERENCES tags(id),
		created_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS config (
		id                 INTEGER PRIMARY KEY CHECK (id = 1),
		device_id          TEXT NOT NULL,
		server_url         TEXT NOT NULL,
		api_key            TEXT NOT NULL,
		tracking_on        INTEGER NOT NULL DEFAULT 1,
		autostart          INTEGER NOT NULL DEFAULT 0,
		presence_intervals TEXT NOT NULL DEFAULT '[30,45,60,90,120]',
		pomodoro_defaults  TEXT NOT NULL DEFAULT '{"work":25,"break":5}'
	);
	`
	_, err := db.Exec(schema)
	return err
}

// OpenBrowser opens the default browser to the given URL.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return exec.Command(cmd, args...).Start()
}
```

- [ ] **23.2** Write tests

```go
// internal/client/setup/firstrun_test.go
package setup

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFirstRun_CreatesDB(t *testing.T) {
	// Use temp dir instead of real data dir
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fr := NewFirstRun(Config{
		ServerURL: "https://test.example.com",
		APIKey:    "test-key",
	}, logger)

	db, err := fr.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	defer db.Close()

	// Verify DB file exists
	dbPath := filepath.Join(tmpDir, "trasker", "trasker.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file not created")
	}

	// Verify config was inserted
	var deviceID, serverURL string
	err = db.QueryRow(`SELECT device_id, server_url FROM config WHERE id = 1`).Scan(&deviceID, &serverURL)
	if err != nil {
		t.Fatalf("query config: %v", err)
	}
	if deviceID == "" {
		t.Error("expected non-empty device_id")
	}
	if serverURL != "https://test.example.com" {
		t.Errorf("expected server URL 'https://test.example.com', got %q", serverURL)
	}
}

func TestFirstRun_IdempotentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	fr := NewFirstRun(Config{ServerURL: "https://test.example.com", APIKey: "key"}, logger)

	db1, err := fr.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var deviceID1 string
	db1.QueryRow(`SELECT device_id FROM config WHERE id = 1`).Scan(&deviceID1)
	db1.Close()

	// Second run should not change device_id (INSERT OR IGNORE)
	db2, err := fr.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	var deviceID2 string
	db2.QueryRow(`SELECT device_id FROM config WHERE id = 1`).Scan(&deviceID2)

	if deviceID1 != deviceID2 {
		t.Errorf("device_id changed on second run: %q vs %q", deviceID1, deviceID2)
	}
}

func TestIsFirstRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	first, err := IsFirstRun()
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Error("expected first run = true before setup")
	}
}

func TestDataDir_Linux(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDir)

	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmpDir, "trasker")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}
```

- [ ] **23.3** Add uuid dependency

```bash
go get github.com/google/uuid
```

- [ ] **23.4** Run tests

```bash
go test ./internal/client/setup/ -v
# Expected: all tests PASS
```

- [ ] **23.5** Commit

```bash
git add internal/client/setup/firstrun.go internal/client/setup/firstrun_test.go
git commit -m "feat(setup): add first-run flow with data dir, schema init, and device ID generation"
```

---

## Task 24: Autostart

**Files:**
- Create: `internal/client/setup/autostart.go`
- Create: `internal/client/setup/autostart_linux.go`
- Create: `internal/client/setup/autostart_linux_test.go`

### Steps

- [ ] **24.1** Create the autostart interface

```go
// internal/client/setup/autostart.go
package setup

// Autostart manages OS-level autostart entries.
type Autostart interface {
	// Enable creates an autostart entry for the current binary.
	Enable() error

	// Disable removes the autostart entry.
	Disable() error

	// IsEnabled checks if autostart is configured.
	IsEnabled() bool
}
```

- [ ] **24.2** Create Linux autostart implementation

```go
// internal/client/setup/autostart_linux.go
//go:build linux

package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopEntry = `[Desktop Entry]
Type=Application
Name=Trasker
Comment=Time tracking daemon
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`

// LinuxAutostart manages ~/.config/autostart/trasker.desktop.
type LinuxAutostart struct {
	execPath string
}

// NewLinuxAutostart creates a Linux autostart manager.
// execPath is the path to the trasker-client binary.
func NewLinuxAutostart(execPath string) *LinuxAutostart {
	return &LinuxAutostart{execPath: execPath}
}

func (a *LinuxAutostart) desktopFilePath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "autostart", "trasker.desktop")
}

// Enable creates the .desktop file for autostart.
func (a *LinuxAutostart) Enable() error {
	path := a.desktopFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("autostart: create dir: %w", err)
	}

	content := fmt.Sprintf(desktopEntry, a.execPath)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("autostart: write desktop file: %w", err)
	}
	return nil
}

// Disable removes the .desktop file.
func (a *LinuxAutostart) Disable() error {
	path := a.desktopFilePath()
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil // already disabled
	}
	if err != nil {
		return fmt.Errorf("autostart: remove desktop file: %w", err)
	}
	return nil
}

// IsEnabled checks if the .desktop file exists.
func (a *LinuxAutostart) IsEnabled() bool {
	_, err := os.Stat(a.desktopFilePath())
	return err == nil
}
```

- [ ] **24.3** Write tests

```go
// internal/client/setup/autostart_linux_test.go
//go:build linux

package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxAutostart_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	a := NewLinuxAutostart("/usr/local/bin/trasker-client")

	if a.IsEnabled() {
		t.Error("expected not enabled initially")
	}

	if err := a.Enable(); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if !a.IsEnabled() {
		t.Error("expected enabled after Enable()")
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(tmpDir, "autostart", "trasker.desktop"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty desktop file")
	}

	if err := a.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if a.IsEnabled() {
		t.Error("expected not enabled after Disable()")
	}
}

func TestLinuxAutostart_DisableWhenNotEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	a := NewLinuxAutostart("/usr/local/bin/trasker-client")

	// Should not error when disabling something that doesn't exist
	if err := a.Disable(); err != nil {
		t.Fatalf("disable: %v", err)
	}
}

func TestLinuxAutostart_ImplementsInterface(t *testing.T) {
	var _ Autostart = (*LinuxAutostart)(nil)
}
```

- [ ] **24.4** Run tests

```bash
go test ./internal/client/setup/ -v -run TestLinuxAutostart
# Expected: all tests PASS
```

- [ ] **24.5** Commit

```bash
git add internal/client/setup/autostart.go internal/client/setup/autostart_linux.go internal/client/setup/autostart_linux_test.go
git commit -m "feat(setup): add Linux autostart via .desktop file"
```

---

## Task 25: Client Main Integration

**Files:**
- Create: `cmd/trasker-client/main.go`

### Steps

- [ ] **25.1** Create the client main entrypoint that wires all components

```go
// cmd/trasker-client/main.go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	_ "modernc.org/sqlite"

	"github.com/jaypaulb/trasker/internal/client/notify"
	"github.com/jaypaulb/trasker/internal/client/pomodoro"
	"github.com/jaypaulb/trasker/internal/client/setup"
	syncpkg "github.com/jaypaulb/trasker/internal/client/sync"
	"github.com/jaypaulb/trasker/internal/client/tagger"
	"github.com/jaypaulb/trasker/internal/client/tray"
	"github.com/jaypaulb/trasker/internal/client/webui"
)

// Build-time values set via ldflags.
var (
	serverURL = "http://localhost:8080"
	apiKey    = "dev-key"
	version   = "dev"
)

const (
	defaultWebPort       = 9746
	learnerThreshold     = 5
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	logger.Info("trasker client starting", "version", version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// First-run setup
	db, err := setupDatabase(ctx, logger)
	if err != nil {
		logger.Error("setup failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Load config
	var deviceID string
	var trackingOn int
	err = db.QueryRow(`SELECT device_id, tracking_on FROM config WHERE id = 1`).Scan(&deviceID, &trackingOn)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// --- Initialize components ---

	// Tagger
	tagApplier := func(eventID, tagID, ruleID int64) error {
		_, err := db.Exec(
			`INSERT OR IGNORE INTO event_tags (event_id, tag_id, source) VALUES (?, ?, 'auto_rule')`,
			eventID, tagID,
		)
		return err
	}
	tg, err := tagger.NewTagger(db, tagApplier, logger, learnerThreshold)
	if err != nil {
		logger.Error("failed to init tagger", "error", err)
		os.Exit(1)
	}

	// Pomodoro
	pomodoroTimer := pomodoro.NewTimer(pomodoro.DefaultConfig())

	// Sync client + queue
	syncClient := syncpkg.NewClient(serverURL, apiKey)
	syncQueue := syncpkg.NewQueue(db, syncClient, deviceID, logger)

	// Register device with server (non-blocking, best effort)
	go func() {
		hostname, _ := os.Hostname()
		err := syncClient.RegisterDevice(ctx, syncpkg.DeviceRegistration{
			ClientDeviceID: deviceID,
			OS:             runtime.GOOS,
			Hostname:       hostname,
		})
		if err != nil {
			logger.Warn("device registration failed (will retry)", "error", err)
		} else {
			logger.Info("device registered with server")
		}
	}()

	// Start submission queue
	syncQueue.Start(ctx)

	// Web UI
	webServer := webui.NewServer(db, defaultWebPort, logger)
	if err := webServer.Start(); err != nil {
		logger.Error("failed to start web server", "error", err)
		os.Exit(1)
	}
	logger.Info("dashboard available at", "url", webServer.URL())

	// Tagger suggestion listener
	go func() {
		for suggestion := range tg.Suggestions() {
			logger.Info("new tag rule suggestion",
				"tag", suggestion.TagName,
				"app", suggestion.AppPattern,
				"title", suggestion.TitlePattern,
				"observed", suggestion.ObservedN,
			)
		}
	}()

	// Pomodoro state change listener
	go func() {
		for change := range pomodoroTimer.Changes() {
			logger.Info("pomodoro state change",
				"from", change.From,
				"to", change.To,
			)
		}
	}()

	// TODO: Wire focus tracker and presence detector from Plan 03
	// The session engine from Plan 03 would feed focus events to:
	// - tg.ProcessFocusEvent(ctx, event)   for auto-tagging
	// - notification dispatcher             for deadman/nag
	// These are wired once Plan 03 components are available.

	// --- System Tray ---
	// Tray must run on the main goroutine (macOS requirement).
	// All other work happens in goroutines above.

	trayActions := &clientTrayActions{
		db:            db,
		webURL:        webServer.URL(),
		pomodoroTimer: pomodoroTimer,
		cancel:        cancel,
		logger:        logger,
	}

	// Run tray in a goroutine if needed, or on main thread
	// On macOS this must be on the main thread. On Linux it can be a goroutine.
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
		webServer.Stop(ctx)
		syncQueue.Stop()
		trayActions.tray.Quit()
	}()

	sysTray := tray.New(trayActions)
	trayActions.tray = sysTray
	sysTray.Run() // blocks until quit

	logger.Info("trasker client stopped")
}

func setupDatabase(ctx context.Context, logger *slog.Logger) (*sql.DB, error) {
	isFirst, err := setup.IsFirstRun()
	if err != nil {
		return nil, fmt.Errorf("check first run: %w", err)
	}

	if isFirst {
		logger.Info("first run detected, initializing...")
		fr := setup.NewFirstRun(setup.Config{
			ServerURL: serverURL,
			APIKey:    apiKey,
		}, logger)
		db, err := fr.Run(ctx)
		if err != nil {
			return nil, err
		}
		// Open browser to dashboard
		go func() {
			setup.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d", defaultWebPort))
		}()
		return db, nil
	}

	// Existing install — just open DB
	dbPath, err := setup.DBPath()
	if err != nil {
		return nil, err
	}
	return sql.Open("sqlite", dbPath)
}

// clientTrayActions implements tray.Actions.
type clientTrayActions struct {
	db            *sql.DB
	webURL        string
	pomodoroTimer *pomodoro.Timer
	cancel        context.CancelFunc
	logger        *slog.Logger
	tray          *tray.Tray

	// Notification integration placeholder
	notifier notify.Notifier
}

func (a *clientTrayActions) OpenDashboard() {
	if err := setup.OpenBrowser(a.webURL); err != nil {
		a.logger.Error("failed to open browser", "error", err)
	}
}

func (a *clientTrayActions) StartPomodoro() {
	if err := a.pomodoroTimer.Start(nil); err != nil {
		a.logger.Warn("pomodoro start failed", "error", err)
	}
}

func (a *clientTrayActions) StopPomodoro() {
	a.pomodoroTimer.Cancel()
}

func (a *clientTrayActions) IsTrackingOn() bool {
	var on int
	a.db.QueryRow(`SELECT tracking_on FROM config WHERE id = 1`).Scan(&on)
	return on == 1
}

func (a *clientTrayActions) SetTrackingOn(on bool) {
	val := 0
	if on {
		val = 1
	}
	a.db.Exec(`UPDATE config SET tracking_on = ? WHERE id = 1`, val)
	a.logger.Info("tracking toggled", "on", on)
}

func (a *clientTrayActions) IsAutostartOn() bool {
	var on int
	a.db.QueryRow(`SELECT autostart FROM config WHERE id = 1`).Scan(&on)
	return on == 1
}

func (a *clientTrayActions) SetAutostartOn(on bool) {
	val := 0
	if on {
		val = 1
	}
	a.db.Exec(`UPDATE config SET autostart = ? WHERE id = 1`, val)

	// Create/remove autostart entry
	execPath, _ := os.Executable()
	autostart := setup.NewAutostart(execPath)
	if on {
		autostart.Enable()
	} else {
		autostart.Disable()
	}
	a.logger.Info("autostart toggled", "on", on)
}

func (a *clientTrayActions) Quit() {
	a.logger.Info("quit requested from tray")
	a.cancel()
}
```

- [ ] **25.2** Verify compilation (will not fully build until Plan 03 provides tracker/presence/session — but the import structure should be sound)

```bash
# Check syntax only — imports may fail until all dependencies exist
go vet ./cmd/trasker-client/ 2>&1 || echo "Expected: may have import errors until Plan 03 is built"
```

- [ ] **25.3** Commit

```bash
git add cmd/trasker-client/main.go
git commit -m "feat(client): add main entrypoint wiring tagger, pomodoro, tray, webui, sync, setup"
```

- [ ] **25.4** Update Makefile with client build target (create or modify)

```makefile
# Makefile (add client target)

.PHONY: client-ui client build-client

# Build Svelte SPA
client-ui:
	cd web/client-ui && npm ci && npm run build
	rm -rf internal/client/webui/static
	cp -r web/client-ui/build internal/client/webui/static

# Build client binary
build-client: client-ui
	go build -ldflags "-X main.version=$(shell git describe --tags --always) \
		-X main.serverURL=$(SERVER_URL) \
		-X main.apiKey=$(API_KEY)" \
		-o bin/trasker-client ./cmd/trasker-client

# Development build (no embedded UI)
build-client-dev:
	go build -o bin/trasker-client ./cmd/trasker-client
```

- [ ] **25.5** Commit Makefile

```bash
git add Makefile
git commit -m "build: add client build targets with embedded SPA and ldflags"
```

---

## Summary

| Task | Package | Key File(s) | Tests |
|------|---------|-------------|-------|
| 1 | tagger | engine.go | engine_test.go |
| 2 | tagger | store.go | store_test.go |
| 3 | tagger | learner.go | learner_test.go |
| 4 | tagger | tagger.go | tagger_test.go |
| 5 | pomodoro | timer.go | timer_test.go |
| 6 | store | pomodoro.go | pomodoro_test.go |
| 7 | tray | tray.go | (integration only) |
| 8 | notify | notify.go | (interface only) |
| 9 | notify | notify_linux.go | interface check |
| 10 | notify | dispatcher.go | dispatcher_test.go |
| 11 | sync | client.go | client_test.go |
| 12 | sync | queue.go | queue_test.go |
| 13 | sync | submit.go | submit_test.go |
| 14 | web/client-ui | SvelteKit scaffold | build check |
| 15 | webui | server.go, embed.go | server_test.go |
| 16 | webui | api.go | api_test.go |
| 17 | web/client-ui | +page.svelte, api.ts, types.ts | build check |
| 18 | web/client-ui | timeline/+page.svelte | build check |
| 19 | web/client-ui | submit/+page.svelte | build check |
| 20 | web/client-ui | pomodoro/+page.svelte | build check |
| 21 | web/client-ui | settings/+page.svelte | build check |
| 22 | web/client-ui | tags/+page.svelte, +layout.svelte | build check |
| 23 | setup | firstrun.go | firstrun_test.go |
| 24 | setup | autostart.go, autostart_linux.go | autostart_linux_test.go |
| 25 | cmd/trasker-client | main.go | compilation check |

**Total commits:** ~25 (one per task)
**Estimated time:** 8-12 hours for an agent working through tasks sequentially
