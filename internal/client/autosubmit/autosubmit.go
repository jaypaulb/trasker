// internal/client/autosubmit/autosubmit.go
//
// Auto-submit: on a fixed cadence, submit every tagged, finished, not-yet-submitted
// focus event whose tag the user has ticked. Submissions are created exactly like a
// manual submit (pending row + submission_events), and the sync queue drains them.
package autosubmit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jaypaulb/trasker/internal/client/sync"
)

// ErrUnknownTag is returned when a ticked tag ID does not exist.
var ErrUnknownTag = errors.New("unknown tag")

// Interval is the set of cadences the UI offers, in hours. 0 means off.
var intervals = map[int]bool{0: true, 1: true, 2: true, 4: true, 6: true, 8: true, 12: true, 24: true}

// ValidInterval reports whether hours is one of the offered cadences.
func ValidInterval(hours int) bool { return intervals[hours] }

// Settings is the persisted auto-submit configuration.
type Settings struct {
	IntervalHours int
	TagIDs        []int64
	// EnabledAt is the cutoff: only events that started at or after it are auto-submitted,
	// so enabling never sweeps up history. Nil when auto-submit is off.
	EnabledAt *time.Time
	// LastRun is the last time a run was performed (or the enable time). Nil when off.
	LastRun *time.Time
}

// Enabled reports whether auto-submit is on.
func (s Settings) Enabled() bool { return s.IntervalHours > 0 && s.EnabledAt != nil }

// NextRun returns when the next run is due, or nil when auto-submit is off.
func (s Settings) NextRun() *time.Time {
	if !s.Enabled() || s.LastRun == nil {
		return nil
	}
	next := s.LastRun.Add(time.Duration(s.IntervalHours) * time.Hour)
	return &next
}

// Load reads the current settings. A missing row means auto-submit has never been set up.
func Load(db *sql.DB) (Settings, error) {
	var s Settings
	var enabledAt, lastRun sql.NullString
	err := db.QueryRow(
		`SELECT interval_hours, enabled_at, last_run FROM auto_submit_config WHERE id = 1`,
	).Scan(&s.IntervalHours, &enabledAt, &lastRun)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Settings{}, fmt.Errorf("auto-submit: load config: %w", err)
	}
	if enabledAt.Valid {
		t, err := time.Parse(time.RFC3339, enabledAt.String)
		if err != nil {
			return Settings{}, fmt.Errorf("auto-submit: parse enabled_at: %w", err)
		}
		s.EnabledAt = &t
	}
	if lastRun.Valid {
		t, err := time.Parse(time.RFC3339, lastRun.String)
		if err != nil {
			return Settings{}, fmt.Errorf("auto-submit: parse last_run: %w", err)
		}
		s.LastRun = &t
	}

	rows, err := db.Query(`SELECT tag_id FROM auto_submit_tags ORDER BY tag_id`)
	if err != nil {
		return Settings{}, fmt.Errorf("auto-submit: load tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return Settings{}, fmt.Errorf("auto-submit: scan tag: %w", err)
		}
		s.TagIDs = append(s.TagIDs, id)
	}
	return s, rows.Err()
}

// Save replaces the settings. Turning auto-submit on (from off) starts the cycle at now:
// the first run is one interval later, and only events starting from now on are eligible.
// Changing the cadence or tags while on keeps the existing cycle.
func Save(db *sql.DB, intervalHours int, tagIDs []int64, now time.Time) error {
	if !ValidInterval(intervalHours) {
		return fmt.Errorf("auto-submit: interval %dh is not offered", intervalHours)
	}
	stamp := now.UTC().Format(time.RFC3339)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("auto-submit: begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO auto_submit_config (id, interval_hours, enabled_at, last_run)
		 VALUES (1, ?, CASE WHEN ? > 0 THEN ? END, CASE WHEN ? > 0 THEN ? END)
		 ON CONFLICT(id) DO UPDATE SET
		   interval_hours = excluded.interval_hours,
		   enabled_at = CASE
		     WHEN excluded.interval_hours = 0 THEN NULL
		     ELSE COALESCE(auto_submit_config.enabled_at, excluded.enabled_at) END,
		   last_run = CASE
		     WHEN excluded.interval_hours = 0 THEN NULL
		     WHEN auto_submit_config.enabled_at IS NULL THEN excluded.last_run
		     ELSE auto_submit_config.last_run END`,
		intervalHours, intervalHours, stamp, intervalHours, stamp,
	); err != nil {
		return fmt.Errorf("auto-submit: save config: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM auto_submit_tags`); err != nil {
		return fmt.Errorf("auto-submit: clear tags: %w", err)
	}
	for _, id := range tagIDs {
		res, err := tx.Exec(`INSERT INTO auto_submit_tags (tag_id) SELECT id FROM tags WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("auto-submit: save tag %d: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: %d", ErrUnknownTag, id)
		}
	}

	return tx.Commit()
}

// RunOnce performs one auto-submit pass if one is due at now. It returns the submission ID
// (0 when nothing was submitted) and the number of events submitted.
func RunOnce(db *sql.DB, now time.Time) (int64, int, error) {
	s, err := Load(db)
	if err != nil {
		return 0, 0, err
	}
	if !s.Enabled() || len(s.TagIDs) == 0 {
		return 0, 0, nil
	}
	if next := s.NextRun(); next != nil && now.Before(*next) {
		return 0, 0, nil
	}

	eventIDs, err := eligibleEvents(db, *s.EnabledAt)
	if err != nil {
		return 0, 0, err
	}

	var subID int64
	if len(eventIDs) > 0 {
		subID, err = sync.NewSubmitService(db).CreateSubmission(eventIDs)
		if err != nil {
			return 0, 0, fmt.Errorf("auto-submit: %w", err)
		}
	}

	if _, err := db.Exec(
		`UPDATE auto_submit_config SET last_run = ? WHERE id = 1`,
		now.UTC().Format(time.RFC3339),
	); err != nil {
		return subID, len(eventIDs), fmt.Errorf("auto-submit: record run: %w", err)
	}
	return subID, len(eventIDs), nil
}

// eligibleEvents returns finished, non-idle events on a ticked tag that started at or after
// the cutoff and are not already part of a submission. Open events are skipped so that
// an entry is never submitted with a partial duration.
func eligibleEvents(db *sql.DB, cutoff time.Time) ([]int64, error) {
	rows, err := db.Query(
		`SELECT DISTINCT fe.id
		 FROM focus_events fe
		 JOIN event_tags et ON et.event_id = fe.id
		 JOIN auto_submit_tags ast ON ast.tag_id = et.tag_id
		 WHERE fe.is_idle = 0
		   AND fe.ended_at IS NOT NULL
		   AND fe.started_at >= ?
		   AND NOT EXISTS (SELECT 1 FROM submission_events se WHERE se.event_id = fe.id)
		 ORDER BY fe.id`,
		cutoff.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("auto-submit: query events: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("auto-submit: scan event: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Run checks for due auto-submit passes once a minute until ctx is cancelled.
// It runs immediately on start so a daemon that was off catches up.
func Run(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		subID, n, err := RunOnce(db, time.Now().UTC())
		if err != nil {
			logger.Error("auto-submit run failed", "error", err)
		} else if subID != 0 {
			logger.Info("auto-submit created submission", "submission_id", subID, "events", n)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
