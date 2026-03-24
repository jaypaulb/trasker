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
	ID        int64
	AppName   string
	StartedAt time.Time
	EndedAt   time.Time
	DurationS int
	Tag       string
	NoteText  string
	NoteTime  *time.Time
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
	return aggregateRawEvents(events)
}

// aggregateRawEvents is the package-level implementation of raw event aggregation.
func aggregateRawEvents(events []RawEvent) []AggregatedEntry {
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

// Aggregate is a package-level function that merges TimesheetEntry slices by tag.
// Used by the Queue's processOne to consolidate entries before submission.
func Aggregate(entries []TimesheetEntry) []TimesheetEntry {
	if len(entries) == 0 {
		return entries
	}

	// Group by tag
	groups := make(map[string][]TimesheetEntry)
	for _, e := range entries {
		groups[e.Tag] = append(groups[e.Tag], e)
	}

	var result []TimesheetEntry
	for tag, group := range groups {
		merged := mergeTimesheetEntries(tag, group)
		result = append(result, merged)
	}

	// Sort by start time for deterministic output
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt < result[j].StartedAt
	})
	return result
}

// mergeTimesheetEntries combines multiple entries with the same tag into one.
func mergeTimesheetEntries(tag string, entries []TimesheetEntry) TimesheetEntry {
	// Sort by start time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartedAt < entries[j].StartedAt
	})

	merged := TimesheetEntry{
		Tag:       tag,
		StartedAt: entries[0].StartedAt,
		EndedAt:   entries[len(entries)-1].EndedAt,
	}

	totalDuration := 0
	var notesParts []string
	for _, e := range entries {
		totalDuration += e.DurationS
		if e.Notes != "" {
			notesParts = append(notesParts, e.Notes)
		}
	}
	merged.DurationS = totalDuration
	merged.Notes = strings.Join(notesParts, "\n")

	return merged
}
