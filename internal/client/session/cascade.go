// internal/client/session/cascade.go
package session

import (
	"fmt"
	"time"

	"github.com/jaypaulb/trasker/internal/client/store"
)

// cascadeSteps defines the half-life decay radii for each cascade step.
// Step 0: ±60 minutes from anchor
// Step 1: ±30 minutes from step 0 edges
// Step 2: ±15 minutes from step 1 edges
// Step 3: ±7.5 minutes from step 2 edges
// Step 4: <5 minutes — stop
var cascadeSteps = []time.Duration{
	60 * time.Minute,
	30 * time.Minute,
	15 * time.Minute,
	450 * time.Second, // 7.5 minutes
	// 5 minutes is the stop threshold — no step 4
}

// Cascader implements the note cascade algorithm.
type Cascader struct {
	store *store.Store
}

// NewCascader creates a new cascade engine.
func NewCascader(s *store.Store) *Cascader {
	return &Cascader{store: s}
}

// Apply runs the cascade algorithm starting from the anchor event.
// It tags the anchor and propagates the tag outward through temporally
// adjacent events with decaying radius. Submitted events are skipped.
// Returns the list of all event IDs that were tagged (including the anchor).
func (c *Cascader) Apply(anchorEventID, tagID int64) ([]int64, error) {
	// Get the anchor event to know its timestamp
	anchor, err := c.store.GetFocusEvent(anchorEventID)
	if err != nil {
		return nil, fmt.Errorf("get anchor event: %w", err)
	}

	// Track all tagged event IDs (to avoid re-processing)
	tagged := make(map[int64]bool)

	// Tag the anchor event as "manual" (user directly acted on it)
	if err := c.store.ApplyTagToEvent(anchorEventID, tagID, "manual", nil); err != nil {
		return nil, fmt.Errorf("tag anchor: %w", err)
	}
	tagged[anchorEventID] = true

	// The "frontier" is the set of events from the current step whose temporal
	// edges define the search window for the next step.
	frontier := []store.FocusEvent{*anchor}

	for stepIdx, radius := range cascadeSteps {
		if len(frontier) == 0 {
			break
		}

		// Find the temporal extremes of the current frontier
		minTime, maxTime := frontierBounds(frontier)

		// Search window: [minTime - radius, maxTime + radius]
		searchFrom := minTime.Add(-radius)
		searchTo := maxTime.Add(radius)

		candidates, err := c.store.ListFocusEvents(searchFrom, searchTo)
		if err != nil {
			return nil, fmt.Errorf("list events for cascade step %d: %w", stepIdx, err)
		}

		var newFrontier []store.FocusEvent
		for _, ev := range candidates {
			if tagged[ev.ID] {
				continue
			}

			// Check if submitted — skip if so
			submitted, err := c.store.IsEventSubmitted(ev.ID)
			if err != nil {
				return nil, fmt.Errorf("check submitted for event %d: %w", ev.ID, err)
			}
			if submitted {
				continue
			}

			// Tag this event
			if err := c.tagEvent(ev.ID, tagID, anchorEventID); err != nil {
				return nil, fmt.Errorf("tag event %d: %w", ev.ID, err)
			}
			tagged[ev.ID] = true
			newFrontier = append(newFrontier, ev)
		}

		frontier = newFrontier
	}

	// Convert tagged map to slice
	result := make([]int64, 0, len(tagged))
	for id := range tagged {
		result = append(result, id)
	}
	return result, nil
}

// tagEvent applies the tag to an event with "cascade" source.
func (c *Cascader) tagEvent(eventID, tagID, anchorID int64) error {
	return c.store.ApplyTagToEvent(eventID, tagID, "cascade", &anchorID)
}

// frontierBounds returns the earliest and latest timestamps in the frontier.
func frontierBounds(events []store.FocusEvent) (time.Time, time.Time) {
	if len(events) == 0 {
		return time.Time{}, time.Time{}
	}

	minT := events[0].StartedAt
	maxT := events[0].StartedAt

	for _, ev := range events[1:] {
		if ev.StartedAt.Before(minT) {
			minT = ev.StartedAt
		}
		// Use ended_at if available for the upper bound
		end := ev.StartedAt
		if ev.EndedAt != nil {
			end = *ev.EndedAt
		}
		if end.After(maxT) {
			maxT = end
		}
	}

	// Also check first event's end time
	if events[0].EndedAt != nil && events[0].EndedAt.After(maxT) {
		maxT = *events[0].EndedAt
	}

	return minT, maxT
}
