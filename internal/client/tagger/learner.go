// internal/client/tagger/learner.go
package tagger

import (
	"database/sql"
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

