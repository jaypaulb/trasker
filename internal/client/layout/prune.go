// Package layout — internal/client/layout/prune.go
//
// Background goroutine that prunes raw layout_snapshots older than the
// configured retention. Per CONTEXT.md D-01, production callers pass
// retention = 7 days and interval = 6 hours.
package layout

import (
	"context"
	"log/slog"
	"time"
)

// RunPrune launches a goroutine that prunes raw layout_snapshots older than
// retention every interval. Per CONTEXT.md D-01, retention = 7 days and
// interval = 6 hours (small enough that prune drift is bounded; large enough
// that the 60s capture loop never contends). Returns when ctx is done.
//
// Three Examples rule applies: this is glue around Store.Prune (which is
// unit-tested in 07-02-PLAN Task 2 / TestStore_Prune7Days). Not a candidate
// for abstraction; no separate test file.
func RunPrune(ctx context.Context, store *Store, retention, interval time.Duration, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	// Run once shortly after start to clear any backlog from a previous session.
	// Use a ctx-aware sleep so shutdown during the warm-up window exits cleanly.
	select {
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
	}

	run := func() {
		deleted, err := store.Prune(retention)
		if err != nil {
			logger.Warn("layout prune failed", "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("layout prune", "deleted_rows", deleted, "retention_hours", retention.Hours())
		}
	}
	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
