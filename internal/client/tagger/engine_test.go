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
