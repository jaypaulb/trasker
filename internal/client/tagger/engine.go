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
